package metrics

import (
	"bytes"
	"sync"
	"unsafe"
)

// nvmlFunc calls one NVML C function with pointer-sized arguments and returns
// its nvmlReturn_t (0 = success). Platform files load the library without
// cgo: LazyDLL on Windows, purego on Linux.
type nvmlFunc func(args ...uintptr) uintptr

// nvml is a minimal binding to NVIDIA's management library, which ships with
// the driver. Only the calls taskman needs are bound.
type nvml struct {
	mu       sync.Mutex
	sym      func(name string) (nvmlFunc, bool)
	cache    map[string]nvmlFunc
	devices  []uintptr
	lastSeen []uint64 // per device: newest process-utilization sample read
}

var (
	nvmlOnce sync.Once
	nvmlLib  *nvml
)

// openNVML loads and initialises NVML once; nil when there's no NVIDIA
// driver.
func openNVML() *nvml {
	nvmlOnce.Do(func() {
		sym, ok := loadNVML()
		if !ok {
			return
		}
		n := &nvml{sym: sym, cache: map[string]nvmlFunc{}}
		if !n.call("nvmlInit_v2") {
			return
		}
		var count uint32
		if !n.call("nvmlDeviceGetCount_v2", ptr(&count)) {
			return
		}
		for i := uint32(0); i < count; i++ {
			var h uintptr
			if n.call("nvmlDeviceGetHandleByIndex_v2", uintptr(i), ptr(&h)) {
				n.devices = append(n.devices, h)
			}
		}
		nvmlLib = n
	})
	return nvmlLib
}

func ptr[T any](p *T) uintptr { return uintptr(unsafe.Pointer(p)) }

func (n *nvml) call(name string, args ...uintptr) bool {
	f, ok := n.cache[name]
	if !ok {
		if f, ok = n.sym(name); !ok {
			return false
		}
		n.cache[name] = f
	}
	return f(args...) == 0
}

// nvmlDevice is what NVML reports for one GPU.
type nvmlDevice struct {
	name      string
	pciDevice uint32 // device ID << 16 | vendor ID
	stat      GPUStat
}

const (
	nvmlTemperatureGPU = 0
	nvmlClockGraphics  = 0
	nvmlClockMem       = 2
)

// read queries every metric NVML offers for each device; missing ones (e.g.
// fan speed on laptops) are simply left unknown.
func (n *nvml) read() []nvmlDevice {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]nvmlDevice, 0, len(n.devices))
	for _, h := range n.devices {
		d := nvmlDevice{stat: GPUStat{Vendor: "NVIDIA", PState: -1}}
		s := &d.stat

		var name [96]byte
		if n.call("nvmlDeviceGetName", h, ptr(&name[0]), uintptr(len(name))) {
			if i := bytes.IndexByte(name[:], 0); i >= 0 {
				d.name = string(name[:i])
			}
		}
		// nvmlPciInfo_t: busIdLegacy[16], domain, bus, device, pciDeviceId, …
		var pci [128]byte
		if n.call("nvmlDeviceGetPciInfo_v3", h, ptr(&pci[0])) {
			d.pciDevice = *(*uint32)(unsafe.Pointer(&pci[28]))
		}

		var util struct{ gpu, mem uint32 }
		if n.call("nvmlDeviceGetUtilizationRates", h, ptr(&util)) {
			s.Util, s.UtilKnown = float64(util.gpu), true
		}
		var enc, dec, period uint32
		if n.call("nvmlDeviceGetEncoderUtilization", h, ptr(&enc), ptr(&period)) {
			s.Engines = append(s.Engines, EngineStat{"Video Encode", float64(enc)})
		}
		if n.call("nvmlDeviceGetDecoderUtilization", h, ptr(&dec), ptr(&period)) {
			s.Engines = append(s.Engines, EngineStat{"Video Decode", float64(dec)})
		}

		var mem struct{ total, free, used uint64 }
		if n.call("nvmlDeviceGetMemoryInfo", h, ptr(&mem)) {
			s.MemTotal, s.MemUsed, s.MemKnown = mem.total, mem.used, true
		}
		var temp uint32
		if n.call("nvmlDeviceGetTemperature", h, nvmlTemperatureGPU, ptr(&temp)) {
			s.TempC, s.TempKnown = float64(temp), true
		}
		var mw, limit uint32
		if n.call("nvmlDeviceGetPowerUsage", h, ptr(&mw)) {
			s.PowerW, s.PowerKnown = float64(mw)/1000, true
		}
		if n.call("nvmlDeviceGetEnforcedPowerLimit", h, ptr(&limit)) {
			s.PowerLimitW = float64(limit) / 1000
		}
		if n.call("nvmlDeviceGetClockInfo", h, nvmlClockGraphics, ptr(&s.ClockMHz)) {
			s.ClockKnown = true
			n.call("nvmlDeviceGetMaxClockInfo", h, nvmlClockGraphics, ptr(&s.ClockMaxMHz))
			n.call("nvmlDeviceGetClockInfo", h, nvmlClockMem, ptr(&s.MemClockMHz))
		}
		if n.call("nvmlDeviceGetFanSpeed", h, ptr(&s.FanPct)) {
			s.FanKnown = true
		}
		var pstate uint32
		if n.call("nvmlDeviceGetPerformanceState", h, ptr(&pstate)) && pstate < 16 {
			s.PState = int(pstate)
		}
		var reasons uint64
		if n.call("nvmlDeviceGetCurrentClocksEventReasons", h, ptr(&reasons)) ||
			n.call("nvmlDeviceGetCurrentClocksThrottleReasons", h, ptr(&reasons)) {
			s.Throttle, s.ThrottleKnown = decodeClockReasons(reasons), true
		}
		out = append(out, d)
	}
	return out
}

// nvmlProcessInfo mirrors nvmlProcessInfo_t, used by the *_v3 process calls.
type nvmlProcessInfo struct {
	pid               uint32
	_                 uint32
	usedGPUMemory     uint64
	gpuInstanceID     uint32
	computeInstanceID uint32
}

// nvmlProcessSample mirrors nvmlProcessUtilizationSample_t.
type nvmlProcessSample struct {
	pid       uint32
	_         uint32
	timeStamp uint64
	smUtil    uint32
	memUtil   uint32
	encUtil   uint32
	decUtil   uint32
}

const nvmlValueNotAvailable = ^uint64(0)

// processes reports per-process VRAM (from the compute and graphics process
// lists) and SM utilization sampled since the previous call.
func (n *nvml) processes() gpuUsage {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.lastSeen == nil {
		n.lastSeen = make([]uint64, len(n.devices))
	}
	out := gpuUsage{}
	for i, h := range n.devices {
		for _, fn := range []string{"nvmlDeviceGetComputeRunningProcesses_v3", "nvmlDeviceGetGraphicsRunningProcesses_v3"} {
			infos := make([]nvmlProcessInfo, 128)
			count := uint32(len(infos))
			if !n.call(fn, h, ptr(&count), ptr(&infos[0])) {
				continue
			}
			for _, p := range infos[:count] {
				u := out[int32(p.pid)]
				if p.usedGPUMemory != nvmlValueNotAvailable {
					u.vram, u.vramKnown = u.vram+p.usedGPUMemory, true
				}
				out[int32(p.pid)] = u
			}
		}
		samples := make([]nvmlProcessSample, 256)
		count := uint32(len(samples))
		if n.call("nvmlDeviceGetProcessUtilization", h, ptr(&samples[0]), ptr(&count), uintptr(n.lastSeen[i])) {
			for _, s := range samples[:count] {
				u := out[int32(s.pid)]
				u.util, u.utilKnown = max(u.util, float64(s.smUtil)), true
				out[int32(s.pid)] = u
				n.lastSeen[i] = max(n.lastSeen[i], s.timeStamp)
			}
		}
	}
	return out
}
