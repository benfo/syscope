package metrics

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
)

func TestSystemCPUPercent(t *testing.T) {
	prev := cpu.TimesStat{User: 10, System: 10, Idle: 80}
	cur := cpu.TimesStat{User: 20, System: 15, Idle: 165, Iowait: 0}
	// delta total 100, delta idle 85 -> 15% busy
	if got := systemCPUPercent(prev, cur); math.Abs(got-15) > 1e-9 {
		t.Errorf("got %v, want 15", got)
	}
	if got := systemCPUPercent(cur, cur); got != 0 {
		t.Errorf("no elapsed time: got %v, want 0", got)
	}
}

func TestKindsFromClasses(t *testing.T) {
	got := kindsFromClasses([]int{1, 1, 0, 0, -1})
	want := []CoreKind{CorePerformance, CorePerformance, CoreEfficiency, CoreEfficiency, CoreUnknown}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hybrid: got %v, want %v", got, want)
	}
	if got := kindsFromClasses([]int{0, 0, 0}); got != nil {
		t.Errorf("uniform CPU should not be hybrid: %v", got)
	}
	if got := kindsFromClasses([]int{-1, -1}); got != nil {
		t.Errorf("unknown classes: %v", got)
	}
}

func TestParseCPUList(t *testing.T) {
	got, err := parseCPUList("0-3,8,10-11\n")
	if want := []int{0, 1, 2, 3, 8, 10, 11}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, %v", got, err)
	}
	for _, bad := range []string{"x", "3-1", "1-y"} {
		if _, err := parseCPUList(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestReadCPUInfo(t *testing.T) {
	info := ReadCPUInfo(context.Background())
	if info.Logical < 1 || info.Model == "" {
		t.Errorf("incomplete: %+v", info)
	}
	if info.Kinds != nil && len(info.Kinds) != info.Logical {
		t.Errorf("kinds length %d != logical %d", len(info.Kinds), info.Logical)
	}
	t.Logf("%s, %d logical, P=%d E=%d", info.Model, info.Logical,
		info.Count(CorePerformance), info.Count(CoreEfficiency))
}
