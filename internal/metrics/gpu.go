package metrics

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// GPUStat is one graphics adapter.
type GPUStat struct {
	ID     string
	Name   string
	Vendor string // "NVIDIA", "AMD", "Intel", …

	Util      float64 // overall busy %, Task Manager style (busiest engine)
	UtilKnown bool
	Engines   []EngineStat // per engine type, when the platform reports them

	MemUsed     uint64 // dedicated VRAM
	MemTotal    uint64
	MemKnown    bool
	SharedUsed  uint64 // system RAM the GPU borrows (Windows)
	SharedTotal uint64

	TempC       float64
	TempKnown   bool
	PowerW      float64
	PowerLimitW float64 // 0 when the driver doesn't report one (common on laptops)
	PowerKnown  bool
	ClockMHz    uint32
	ClockMaxMHz uint32
	MemClockMHz uint32
	ClockKnown  bool
	FanPct      uint32
	FanKnown    bool
	FanRPM      uint32  // from Windows D3DKMT when NVML has no fan reading
	PowerPct    float64 // share of max power, for drivers that don't report watts
	PState      int     // 0 = max performance … 15 = idle; -1 unknown

	Throttle      []string // active clock-limit reasons
	ThrottleKnown bool
}

// EngineStat is one engine type, e.g. "3D", "Copy", "Video Decode".
type EngineStat struct {
	Name string
	Util float64
}

// clockEventReasons are NVML's nvmlClocksEventReason* bits.
var clockEventReasons = []struct {
	bit  uint64
	name string
}{
	{0x1, "idle"},
	{0x2, "application clocks"},
	{0x4, "power cap"},
	{0x8, "hardware slowdown"},
	{0x10, "sync boost"},
	{0x20, "thermal (software)"},
	{0x40, "thermal (hardware)"},
	{0x80, "power brake"},
	{0x100, "display clock"},
	{0x400, "reliability"},
}

// decodeClockReasons names the set bits; unknown bits are shown in hex.
func decodeClockReasons(mask uint64) []string {
	var out []string
	for _, r := range clockEventReasons {
		if mask&r.bit != 0 {
			out = append(out, r.name)
			mask &^= r.bit
		}
	}
	if mask != 0 {
		out = append(out, fmt.Sprintf("0x%x", mask))
	}
	return out
}

// vendorName maps a PCI vendor ID to a display name.
func vendorName(id uint32) string {
	switch id {
	case 0x10DE:
		return "NVIDIA"
	case 0x1002, 0x1022:
		return "AMD"
	case 0x8086:
		return "Intel"
	case 0x5143:
		return "Qualcomm"
	case 0x106B:
		return "Apple"
	}
	return ""
}

var engineInstanceRE = regexp.MustCompile(`(?i)luid_(0x[0-9a-f]+_0x[0-9a-f]+)_phys_\d+_eng_(\d+)_engtype_(.*)$`)

// aggregateEngines turns Windows "GPU Engine" counter instances
// (pid_<pid>_luid_<hi>_<lo>_phys_0_eng_<n>_engtype_<type>) into utilization
// per adapter LUID and engine type, the way Task Manager does: processes are
// summed per physical engine, and an engine type shows its busiest engine.
func aggregateEngines(vals map[string]float64) map[string]map[string]float64 {
	type engKey struct{ luid, idx, typ string }
	perEngine := map[engKey]float64{}
	for inst, v := range vals {
		m := engineInstanceRE.FindStringSubmatch(inst)
		if m == nil {
			continue
		}
		perEngine[engKey{strings.ToLower(m[1]), m[2], m[3]}] += v
	}
	out := map[string]map[string]float64{}
	for k, v := range perEngine {
		if out[k.luid] == nil {
			out[k.luid] = map[string]float64{}
		}
		out[k.luid][k.typ] = max(out[k.luid][k.typ], min(v, 100))
	}
	return out
}

// engineOrder lists the engines people care about first.
var engineOrder = []string{"3D", "Compute", "Copy", "Video Decode", "Video Encode"}

// engineStats converts per-type utilization to a sorted list with readable
// names ("VideoDecode" → "Video Decode"), and returns the busiest value.
func engineStats(types map[string]float64) ([]EngineStat, float64) {
	byName := map[string]float64{}
	busiest := 0.0
	for typ, v := range types {
		if name := prettyEngine(typ); name != "" {
			byName[name] = max(byName[name], v) // e.g. Security and Security_1
		}
		busiest = max(busiest, v)
	}
	out := make([]EngineStat, 0, len(byName))
	for name, v := range byName {
		out = append(out, EngineStat{Name: name, Util: v})
	}
	rank := func(name string) int {
		for i, n := range engineOrder {
			if n == name {
				return i
			}
		}
		return len(engineOrder)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := rank(out[i].Name), rank(out[j].Name)
		if ri != rj {
			return ri < rj
		}
		return out[i].Name < out[j].Name
	})
	return out, busiest
}

var engineIndexRE = regexp.MustCompile(`_\d+$`)

// prettyEngine makes engine type names readable: "VideoDecode" → "Video
// Decode", "JPEG_Decode_0" → "JPEG Decode", "3D" stays "3D".
func prettyEngine(typ string) string {
	typ = strings.ReplaceAll(engineIndexRE.ReplaceAllString(typ, ""), "_", " ")
	var b strings.Builder
	prev := ' '
	for _, r := range typ {
		if unicode.IsUpper(r) && unicode.IsLower(prev) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		prev = r
	}
	return strings.TrimSpace(b.String())
}

// luidKey formats an adapter LUID the way PDH instance names do.
func luidKey(high int32, low uint32) string {
	return fmt.Sprintf("0x%08x_0x%08x", uint32(high), low)
}

// gpuCollector reads every GPU. gpuState is the platform's between-sample
// state (Windows keeps its performance-counter query open).
type gpuCollector struct{ state gpuState }

func (c *gpuCollector) collect(_ context.Context, _ time.Time, snap *SystemSnapshot) error {
	snap.GPUs = c.state.read()
	return nil
}
