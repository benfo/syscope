package metrics

import (
	"fmt"
	"strings"
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
	PState      int // 0 = max performance … 15 = idle; -1 unknown

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

// shortGPUName trims vendor boilerplate for narrow places like the sidebar:
// "NVIDIA GeForce RTX 3080 Ti Laptop GPU" → "RTX 3080 Ti Laptop".
func shortGPUName(name string) string {
	r := strings.NewReplacer("NVIDIA ", "", "GeForce ", "", "(R)", "", "(TM)", "", "AMD ", "", "Radeon(TM) ", "")
	return strings.TrimSuffix(strings.TrimSpace(r.Replace(name)), " GPU")
}
