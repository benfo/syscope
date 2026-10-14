package metrics

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDecodeClockReasons(t *testing.T) {
	got := decodeClockReasons(0x1 | 0x4 | 0x400 | 0x8000)
	if want := []string{"idle", "power cap", "reliability", "0x8000"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := decodeClockReasons(0); got != nil {
		t.Errorf("no reasons: %v", got)
	}
}

func TestNVMLLive(t *testing.T) {
	n := openNVML()
	if n == nil {
		t.Skip("no NVIDIA driver")
	}
	for _, d := range n.read() {
		s := d.stat
		t.Logf("%q pci=0x%08X util=%.0f(%v) mem=%d/%d temp=%.0f power=%.1fW limit=%.1fW clock=%d/%d mem=%d fan=%v pstate=P%d throttle=%v engines=%v",
			d.name, d.pciDevice, s.Util, s.UtilKnown, s.MemUsed>>20, s.MemTotal>>20, s.TempC, s.PowerW, s.PowerLimitW,
			s.ClockMHz, s.ClockMaxMHz, s.MemClockMHz, s.FanKnown, s.PState, s.Throttle, s.Engines)
		if d.name == "" || !s.MemKnown || s.MemTotal == 0 {
			t.Errorf("incomplete NVML device: %+v", d)
		}
	}
}

func TestAggregateEngines(t *testing.T) {
	vals := map[string]float64{
		// Two processes on 3D engine 0, one on 3D engine 1: engine 0 = 50, type max = 50.
		"pid_100_luid_0x00000000_0x000142DE_phys_0_eng_0_engtype_3D":          30,
		"pid_200_luid_0x00000000_0x000142DE_phys_0_eng_0_engtype_3D":          20,
		"pid_300_luid_0x00000000_0x000142DE_phys_0_eng_1_engtype_3D":          10,
		"pid_100_luid_0x00000000_0x000142DE_phys_0_eng_5_engtype_VideoDecode": 7,
		"pid_100_luid_0x00000000_0x00013E7A_phys_0_eng_0_engtype_3D":          80,
		"garbage": 5,
	}
	agg := aggregateEngines(vals)
	nv := agg[luidKey(0, 0x142DE)]
	if nv["3D"] != 50 || nv["VideoDecode"] != 7 {
		t.Errorf("nvidia engines: %v", nv)
	}
	stats, busiest := engineStats(nv)
	if busiest != 50 || stats[0].Name != "3D" || stats[1].Name != "Video Decode" {
		t.Errorf("engineStats: %v %v", stats, busiest)
	}
	if len(agg) != 2 {
		t.Errorf("want 2 adapters, got %d", len(agg))
	}
}

func TestReadAMDFakeSysfs(t *testing.T) {
	root := t.TempDir()
	write := func(rel, val string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(val+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dev := "class/drm/card1/device/"
	write(dev+"vendor", "0x1002")
	write(dev+"product_name", "AMD Radeon RX 7900 XTX")
	write(dev+"gpu_busy_percent", "42")
	write(dev+"mem_info_vram_used", "4294967296")
	write(dev+"mem_info_vram_total", "25769803776")
	write(dev+"hwmon/hwmon3/temp1_input", "61000")
	write(dev+"hwmon/hwmon3/power1_average", "187000000")
	write(dev+"hwmon/hwmon3/power1_cap", "355000000")
	write(dev+"hwmon/hwmon3/freq1_input", "2498000000")
	write(dev+"hwmon/hwmon3/fan1_input", "1450")
	write("class/drm/card0/device/vendor", "0x8086")  // Intel: skipped
	write("class/drm/card1-DP-1/status", "connected") // connector: skipped

	got := readAMD(root)
	if len(got) != 1 {
		t.Fatalf("want 1 AMD GPU, got %d", len(got))
	}
	g := got[0]
	if g.Name != "AMD Radeon RX 7900 XTX" || g.Util != 42 || g.MemUsed != 4<<30 || g.MemTotal != 24<<30 ||
		g.TempC != 61 || g.PowerW != 187 || g.PowerLimitW != 355 || g.ClockMHz != 2498 || g.FanRPM != 1450 {
		t.Errorf("AMD parse: %+v", g)
	}
}

func TestGPUsLive(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	s.Sample(ctx)
	time.Sleep(500 * time.Millisecond)
	snap, _, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.GPUs) == 0 {
		t.Skip("no GPUs visible")
	}
	for _, g := range snap.GPUs {
		t.Logf("GPU %s %q %s util=%.1f(%v) engines=%v mem=%dMiB/%dMiB(%v) shared=%dMiB/%dMiB temp=%.1f(%v) power=%.1f/%.0fW pct=%.1f clock=%d/%d P%d throttle=%v",
			g.ID, g.Name, g.Vendor, g.Util, g.UtilKnown, g.Engines, g.MemUsed>>20, g.MemTotal>>20, g.MemKnown,
			g.SharedUsed>>20, g.SharedTotal>>20, g.TempC, g.TempKnown, g.PowerW, g.PowerLimitW, g.PowerPct,
			g.ClockMHz, g.ClockMaxMHz, g.PState, g.Throttle)
		if g.Name == "" || g.MemTotal == 0 {
			t.Errorf("incomplete GPU: %+v", g)
		}
	}
}

func TestPrettyEngine(t *testing.T) {
	for in, want := range map[string]string{
		"3D": "3D", "VideoDecode": "Video Decode", "JPEG_Decode_0": "JPEG Decode", "OFA_0": "OFA",
		"Security_1": "Security", "GDI Render": "GDI Render", "High Priority 3D": "High Priority 3D", "": "",
	} {
		if got := prettyEngine(in); got != want {
			t.Errorf("prettyEngine(%q) = %q, want %q", in, got, want)
		}
	}
}
