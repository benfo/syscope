package metrics

import (
	"encoding/binary"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	gpuEngineCounter    = `\GPU Engine(*)\Utilization Percentage`
	gpuDedicatedCounter = `\GPU Adapter Memory(*)\Dedicated Usage`
	gpuSharedCounter    = `\GPU Adapter Memory(*)\Shared Usage`
	gpuProcessVRAM      = `\GPU Process Memory(*)\Dedicated Usage`
	adapterRefresh      = 30 * time.Second
)

// gpuState keeps the PDH query open between samples (utilization is a rate)
// and caches the adapter list, which rarely changes.
type gpuState struct {
	query      *pdhQuery
	adapters   []dxgiAdapter
	adaptersAt time.Time
}

func (g *gpuState) read() ([]GPUStat, gpuUsage) {
	if g.adapters == nil || time.Since(g.adaptersAt) > adapterRefresh {
		g.adapters, g.adaptersAt = dxgiAdapters(), time.Now()
	}
	if len(g.adapters) == 0 {
		return nil, nil
	}
	if g.query == nil {
		if q, err := newPDHQuery(gpuEngineCounter, gpuDedicatedCounter, gpuSharedCounter, gpuProcessVRAM); err == nil {
			g.query = q
			q.collect() // prime the rate counters
		}
	}
	var engineVals, dedicated, shared, processVRAM map[string]float64
	if g.query != nil && g.query.collect() == nil {
		engineVals = g.query.values(gpuEngineCounter)
		dedicated = g.query.values(gpuDedicatedCounter)
		shared = g.query.values(gpuSharedCounter)
		processVRAM = g.query.values(gpuProcessVRAM)
	}
	engines := aggregateEngines(engineVals)

	var nv []nvmlDevice
	if n := openNVML(); n != nil {
		nv = n.read()
	}
	used := make([]bool, len(nv))

	out := make([]GPUStat, 0, len(g.adapters))
	for i, a := range g.adapters {
		key := luidKey(a.luidHigh, a.luidLow)
		st := GPUStat{ID: strconv.Itoa(i), Name: a.name, Vendor: vendorName(a.vendor), PState: -1,
			MemTotal: a.dedicated, SharedTotal: a.shared}
		if types, ok := engines[key]; ok {
			st.Engines, st.Util = engineStats(types)
			st.UtilKnown = true
		}
		if v, ok := sumPrefix(dedicated, "luid_"+key); ok {
			st.MemUsed, st.MemKnown = uint64(v), true
		}
		if v, ok := sumPrefix(shared, "luid_"+key); ok {
			st.SharedUsed = uint64(v)
		}
		// NVIDIA extras: match the NVML device with the same PCI device ID.
		for j, d := range nv {
			if !used[j] && a.vendor == 0x10DE && d.pciDevice>>16 == a.device {
				used[j] = true
				mergeNVML(&st, d.stat)
				break
			}
		}
		// Any vendor: driver-reported sensors fill what NVML didn't.
		if p, ok := queryAdapterPerf(a); ok {
			mergeAdapterPerf(&st, p)
		}
		out = append(out, st)
	}
	return out, perProcessGPU(engineVals, processVRAM)
}

// mergeNVML adds NVML-only metrics to a Windows adapter; usage and memory
// stay from PDH so they match Task Manager.
func mergeNVML(st *GPUStat, n GPUStat) {
	st.TempC, st.TempKnown = n.TempC, n.TempKnown
	st.PowerW, st.PowerLimitW, st.PowerKnown = n.PowerW, n.PowerLimitW, n.PowerKnown
	st.ClockMHz, st.ClockMaxMHz, st.MemClockMHz, st.ClockKnown = n.ClockMHz, n.ClockMaxMHz, n.MemClockMHz, n.ClockKnown
	st.FanPct, st.FanKnown = n.FanPct, n.FanKnown
	st.PState = n.PState
	st.Throttle, st.ThrottleKnown = n.Throttle, n.ThrottleKnown
	if !st.MemKnown {
		st.MemUsed, st.MemTotal, st.MemKnown = n.MemUsed, n.MemTotal, n.MemKnown
	}
	if !st.UtilKnown {
		st.Util, st.UtilKnown = n.Util, n.UtilKnown
	}
}

func sumPrefix(vals map[string]float64, prefix string) (float64, bool) {
	var sum float64
	found := false
	for inst, v := range vals {
		if strings.HasPrefix(strings.ToLower(inst), prefix) {
			sum += v
			found = true
		}
	}
	return sum, found
}

type dxgiAdapter struct {
	name      string
	vendor    uint32
	device    uint32
	dedicated uint64
	shared    uint64
	luidLow   uint32
	luidHigh  int32
}

var (
	procCreateDXGIFactory1 = windows.NewLazySystemDLL("dxgi.dll").NewProc("CreateDXGIFactory1")
	iidIDXGIFactory1       = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba,
		Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
)

// comObject is a COM interface pointer: its first field is the vtable.
type comObject struct{ vtbl *[32]uintptr }

const (
	vtblRelease       = 2
	vtblEnumAdapters1 = 12 // IUnknown(3) + IDXGIObject(4) + IDXGIFactory(5)
	vtblGetDesc1      = 10 // IUnknown(3) + IDXGIObject(4) + IDXGIAdapter(3)
	dxgiFlagSoftware  = 2
	vendorMicrosoft   = 0x1414 // Basic Render, remote/indirect display adapters
)

func (o *comObject) call(method int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[method], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

// dxgiAdapters lists hardware GPUs via DXGI (no COM initialisation needed).
func dxgiAdapters() []dxgiAdapter {
	var factory *comObject
	if r, _, _ := procCreateDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)),
		uintptr(unsafe.Pointer(&factory))); r != 0 || factory == nil {
		return nil
	}
	defer factory.call(vtblRelease)

	le := binary.LittleEndian
	var out []dxgiAdapter
	for i := uintptr(0); ; i++ {
		var adapter *comObject
		if factory.call(vtblEnumAdapters1, i, uintptr(unsafe.Pointer(&adapter))) != 0 || adapter == nil {
			break // DXGI_ERROR_NOT_FOUND: no more adapters
		}
		// DXGI_ADAPTER_DESC1: Description[128]WCHAR, VendorId, DeviceId, SubSysId,
		// Revision, DedicatedVideoMemory, DedicatedSystemMemory, SharedSystemMemory
		// (SIZE_T), AdapterLuid{Low, High}, Flags.
		var desc [312]byte
		ok := adapter.call(vtblGetDesc1, uintptr(unsafe.Pointer(&desc[0]))) == 0
		adapter.call(vtblRelease)
		if !ok {
			continue
		}
		vendor := le.Uint32(desc[256:])
		if le.Uint32(desc[304:])&dxgiFlagSoftware != 0 || vendor == vendorMicrosoft {
			continue
		}
		name := make([]uint16, 128)
		for j := range name {
			name[j] = le.Uint16(desc[j*2:])
		}
		out = append(out, dxgiAdapter{
			name:      strings.TrimSpace(windows.UTF16ToString(name)),
			vendor:    vendor,
			device:    le.Uint32(desc[260:]),
			dedicated: le.Uint64(desc[272:]),
			shared:    le.Uint64(desc[288:]),
			luidLow:   le.Uint32(desc[296:]),
			luidHigh:  int32(le.Uint32(desc[300:])),
		})
	}
	return out
}

var (
	gdi32                         = windows.NewLazySystemDLL("gdi32.dll")
	procD3DKMTOpenAdapterFromLuid = gdi32.NewProc("D3DKMTOpenAdapterFromLuid")
	procD3DKMTQueryAdapterInfo    = gdi32.NewProc("D3DKMTQueryAdapterInfo")
	procD3DKMTCloseAdapter        = gdi32.NewProc("D3DKMTCloseAdapter")
)

const kmtqaiAdapterPerfData = 62 // KMTQAITYPE_ADAPTERPERFDATA

// adapterPerf is the vendor-neutral sensor data WDDM 2.4+ drivers report,
// which Task Manager uses for GPU temperature.
type adapterPerf struct {
	tempC    float64
	fanRPM   uint32
	powerPct float64 // share of the adapter's max power
}

// queryAdapterPerf reads D3DKMT_ADAPTER_PERFDATA. Zero values mean the
// driver doesn't report that sensor (common on integrated GPUs).
func queryAdapterPerf(a dxgiAdapter) (adapterPerf, bool) {
	// D3DKMT_OPENADAPTERFROMLUID: LUID{Low uint32, High int32}, hAdapter uint32.
	var open struct {
		luidLow  uint32
		luidHigh int32
		handle   uint32
	}
	open.luidLow, open.luidHigh = a.luidLow, a.luidHigh
	if r, _, _ := procD3DKMTOpenAdapterFromLuid.Call(uintptr(unsafe.Pointer(&open))); r != 0 {
		return adapterPerf{}, false
	}
	defer procD3DKMTCloseAdapter.Call(uintptr(unsafe.Pointer(&open.handle)))

	// D3DKMT_ADAPTER_PERFDATA: PhysicalAdapterIndex uint32, then (8-aligned)
	// MemoryFrequency, MaxMemoryFrequency, MaxMemoryFrequencyOC, MemoryBandwidth,
	// PCIEBandwidth uint64, FanRPM, Power (0.1 %), Temperature (0.1 °C) uint32,
	// PowerStateOverride byte.
	var data [64]byte
	query := struct {
		handle uint32
		typ    uint32
		data   unsafe.Pointer
		size   uint32
	}{open.handle, kmtqaiAdapterPerfData, unsafe.Pointer(&data[0]), uint32(len(data))}
	if r, _, _ := procD3DKMTQueryAdapterInfo.Call(uintptr(unsafe.Pointer(&query))); r != 0 {
		return adapterPerf{}, false
	}
	le := binary.LittleEndian
	return adapterPerf{
		fanRPM:   le.Uint32(data[48:]),
		powerPct: float64(le.Uint32(data[52:])) / 10,
		tempC:    float64(le.Uint32(data[56:])) / 10,
	}, true
}

// mergeAdapterPerf fills sensors NVML didn't provide. Drivers report 0 for
// sensors they don't have, so zeros stay unknown.
func mergeAdapterPerf(st *GPUStat, p adapterPerf) {
	if !st.TempKnown && p.tempC > 0 {
		st.TempC, st.TempKnown = p.tempC, true
	}
	if !st.FanKnown && p.fanRPM > 0 {
		st.FanRPM = p.fanRPM
	}
	if !st.PowerKnown && p.powerPct > 0 {
		st.PowerPct = p.powerPct
	}
}
