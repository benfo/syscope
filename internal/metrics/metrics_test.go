package metrics

import (
	"context"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

func TestProcCPUPercent(t *testing.T) {
	tests := []struct {
		name      string
		prev, cur float64
		wall      time.Duration
		numCPU    int
		want      float64
	}{
		{"two of eight cores saturated", 10, 12, time.Second, 8, 25},
		{"one core on one-core box", 0, 1, time.Second, 1, 100},
		{"idle", 5, 5, time.Second, 4, 0},
		{"clock went backwards", 5, 4, time.Second, 4, 0},
		{"zero window", 0, 1, 0, 4, 0},
		{"clamped", 0, 100, time.Second, 2, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := procCPUPercent(tt.prev, tt.cur, tt.wall, tt.numCPU)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

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

func TestStaleEntry(t *testing.T) {
	if !staleEntry(1000, 2000) {
		t.Error("different create times should be stale (PID reused)")
	}
	if staleEntry(1000, 1000) {
		t.Error("same create time should not be stale")
	}
	if staleEntry(0, 2000) || staleEntry(1000, 0) {
		t.Error("unknown create time should not force a refresh")
	}
}

func TestGopsutilSamplerFindsSelf(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	if _, _, err := s.Sample(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	snap, procs, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.CPUKnown || snap.MemTotal == 0 || snap.NumCPU < 1 {
		t.Errorf("bad system snapshot: %+v", snap)
	}
	self := int32(os.Getpid())
	for _, p := range procs {
		if p.PID == self {
			if p.Name == "" || !p.CPUKnown || !p.RSSKnown || p.RSS == 0 {
				t.Errorf("own process incomplete: %+v", p)
			}
			return
		}
	}
	t.Errorf("own PID %d not found among %d processes", self, len(procs))
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

func TestDiskDelta(t *testing.T) {
	// Windows style: 100ns ticks, busy measured against the query clock.
	prev := diskRaw{ReadBytes: 1000, WriteBytes: 0, Reads: 10, Writes: 0,
		TickUnit: 100 * time.Nanosecond, IOTime: 0, Busy: 0, Clock: 1e7, HasBusy: true, HasIO: true}
	cur := prev
	cur.ReadBytes, cur.WriteBytes = 1000+4_000_000, 2_000_000
	cur.Reads, cur.Writes = 30, 10            // 30 I/Os
	cur.IOTime = 30 * 5_000                   // 0.5 ms each
	cur.Clock, cur.Busy = 2e7, 2_500_000      // 0.25s busy of 1s
	st := diskDelta(prev, cur, 2*time.Second) // wall ignored for active when Clock is set
	if st.ReadBps != 2_000_000 || st.WriteBps != 1_000_000 {
		t.Errorf("rates: %+v", st)
	}
	if !st.ActiveKnown || math.Abs(st.ActivePct-25) > 1e-9 {
		t.Errorf("active: %v %v", st.ActivePct, st.ActiveKnown)
	}
	if !st.RespKnown || math.Abs(st.RespMs-0.5) > 1e-9 {
		t.Errorf("response: %v", st.RespMs)
	}

	// Linux style: ms ticks against wall time; counter reset reads as zero.
	lp := diskRaw{TickUnit: time.Millisecond, Busy: 1000, ReadBytes: 500, HasBusy: true}
	lc := lp
	lc.Busy, lc.ReadBytes = 1500, 100
	st = diskDelta(lp, lc, time.Second)
	if math.Abs(st.ActivePct-50) > 1e-9 || st.ReadBps != 0 || st.RespKnown {
		t.Errorf("linux style: %+v", st)
	}
}

func TestParentDisk(t *testing.T) {
	disks := []string{"nvme0n1", "nvme0n10", "sda"}
	for part, want := range map[string]string{
		"nvme0n1p2": "nvme0n1", "nvme0n10p1": "nvme0n10", "sda1": "sda", "sdb1": "", "sda": "sda",
	} {
		if got := parentDisk(part, disks); got != want {
			t.Errorf("parentDisk(%q) = %q, want %q", part, got, want)
		}
	}
}

func TestReadDisks(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	s.Sample(ctx)
	time.Sleep(300 * time.Millisecond)
	snap, _, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Disks) == 0 {
		t.Skip("no physical disks visible (container?)")
	}
	for _, d := range snap.Disks {
		if !d.RatesKnown {
			t.Errorf("%s: rates unknown on second sample", d.Name)
		}
		t.Logf("%s %q %s active=%.1f%%(%v) r=%.0f w=%.0f resp=%.2fms vols=%+v",
			d.Name, d.Model, d.Bus, d.ActivePct, d.ActiveKnown, d.ReadBps, d.WriteBps, d.RespMs, d.Volumes)
	}
}

func TestDefaultVisible(t *testing.T) {
	cases := []struct {
		up, loop, phys, seen, want bool
	}{
		{true, false, true, false, true},   // Wi-Fi, idle
		{true, false, false, true, true},   // VPN with traffic
		{true, false, false, false, false}, // idle Hyper-V switch
		{false, false, true, true, false},  // unplugged Ethernet
		{true, true, false, true, false},   // loopback
	}
	for _, c := range cases {
		if got := defaultVisible(c.up, c.loop, c.phys, c.seen); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestPickAddrs(t *testing.T) {
	mk := func(s string) net.Addr {
		ip, ipn, _ := net.ParseCIDR(s)
		ipn.IP = ip
		return ipn
	}
	got := pickAddrs([]net.Addr{mk("fe80::1/64"), mk("2001:db8::5/64"), mk("192.168.1.9/24"), mk("10.0.0.1/8")})
	if want := []string{"192.168.1.9", "2001:db8::5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSortNets(t *testing.T) {
	ns := []NetStat{{Name: "vEthernet", Default: true}, {Name: "Ethernet", Physical: true}, {Name: "Wi-Fi", Physical: true, Default: true}}
	sortNets(ns)
	if ns[0].Name != "Wi-Fi" || ns[1].Name != "vEthernet" || ns[2].Name != "Ethernet" {
		t.Errorf("order: %v %v %v", ns[0].Name, ns[1].Name, ns[2].Name)
	}
}

func TestSampleNets(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	s.Sample(ctx)
	time.Sleep(300 * time.Millisecond)
	snap, _, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Nets) == 0 {
		t.Fatal("no interfaces")
	}
	for _, n := range snap.Nets {
		t.Logf("%-34q default=%-5v phys=%-5v up=%-5v speed=%d rx=%.0f tx=%.0f %v %q",
			n.Name, n.Default, n.Physical, n.Up, n.SpeedBps, n.RecvBps, n.SentBps, n.Addrs, n.Desc)
	}
}

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
