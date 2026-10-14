package metrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
)

// CoreKind distinguishes performance and efficiency cores on hybrid CPUs
// (Intel P/E cores, ARM big.LITTLE).
type CoreKind uint8

const (
	CoreUnknown CoreKind = iota
	CorePerformance
	CoreEfficiency
)

// Letter is "P", "E" or "" for display.
func (k CoreKind) Letter() string {
	switch k {
	case CorePerformance:
		return "P"
	case CoreEfficiency:
		return "E"
	}
	return ""
}

// CPUInfo is static processor information, read once at startup.
type CPUInfo struct {
	Model    string
	BaseMHz  float64
	Physical int
	Logical  int
	// Kinds has one entry per logical CPU in the same order as
	// SystemSnapshot.CPUPerCore; nil when the CPU is not hybrid or the
	// platform can't tell.
	Kinds []CoreKind
}

// Count returns how many logical CPUs are of kind k.
func (c CPUInfo) Count(k CoreKind) int {
	n := 0
	for _, kk := range c.Kinds {
		if kk == k {
			n++
		}
	}
	return n
}

func ReadCPUInfo(ctx context.Context) CPUInfo {
	var info CPUInfo
	if is, err := cpu.InfoWithContext(ctx); err == nil && len(is) > 0 {
		info.Model = strings.TrimSpace(is[0].ModelName)
		info.BaseMHz = is[0].Mhz
	}
	info.Logical, _ = cpu.CountsWithContext(ctx, true)
	info.Physical, _ = cpu.CountsWithContext(ctx, false)
	info.Kinds = coreKinds(info.Logical)
	return info
}

// kindsFromClasses maps per-CPU performance classes (higher = faster, -1 =
// unknown) to core kinds. Returns nil when every known class is equal, i.e.
// the CPU is not hybrid.
func kindsFromClasses(classes []int) []CoreKind {
	lo, hi := -1, -1
	for _, c := range classes {
		if c < 0 {
			continue
		}
		if lo < 0 || c < lo {
			lo = c
		}
		if c > hi {
			hi = c
		}
	}
	if hi < 0 || lo == hi {
		return nil
	}
	kinds := make([]CoreKind, len(classes))
	for i, c := range classes {
		switch {
		case c == hi:
			kinds[i] = CorePerformance
		case c >= 0:
			kinds[i] = CoreEfficiency
		}
	}
	return kinds
}

// parseCPUList parses Linux CPU list syntax such as "0-7,16,18-19".
func parseCPUList(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(strings.TrimSpace(s), ",") {
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			return nil, fmt.Errorf("bad cpu list %q", s)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil || b < a {
				return nil, fmt.Errorf("bad cpu list %q", s)
			}
		}
		for i := a; i <= b; i++ {
			out = append(out, i)
		}
	}
	return out, nil
}
