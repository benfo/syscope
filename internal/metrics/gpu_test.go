package metrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
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

func TestPerProcessGPU(t *testing.T) {
	engines := map[string]float64{
		"pid_100_luid_0x00000000_0x000142de_phys_0_eng_0_engtype_3D":          40,
		"pid_100_luid_0x00000000_0x000142de_phys_0_eng_5_engtype_VideoDecode": 70,
		"pid_200_luid_0x00000000_0x00013e7a_phys_0_eng_0_engtype_3D":          2,
		"luid_0x00000000_0x000142de_phys_0_eng_0_engtype_3D":                  99, // no pid: ignored
	}
	vram := map[string]float64{
		"pid_100_luid_0x00000000_0x000142de_phys_0": 7 << 30,
		"pid_100_luid_0x00000000_0x00013e7a_phys_0": 1 << 20, // also on the iGPU: summed
		"pid_300_luid_0x00000000_0x000142de_phys_0": 512 << 20,
	}
	got := perProcessGPU(engines, vram)
	if u := got[100]; u.util != 70 || !u.utilKnown || u.vram != 7<<30+1<<20 || !u.vramKnown {
		t.Errorf("pid 100: %+v", u)
	}
	if u := got[300]; u.utilKnown || u.vram != 512<<20 {
		t.Errorf("pid 300 (VRAM only): %+v", u)
	}
	if len(got) != 3 {
		t.Errorf("want 3 pids, got %d", len(got))
	}
}

func TestPerProcessGPULive(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	s.Sample(ctx)
	time.Sleep(500 * time.Millisecond)
	_, procs, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].VRAM > procs[j].VRAM })
	n := 0
	for _, p := range procs {
		if p.VRAMKnown || p.GPUKnown {
			n++
			if n <= 6 {
				t.Logf("%6d %-28s gpu=%5.1f(%v) vram=%6d MiB", p.PID, p.Name, p.GPU, p.GPUKnown, p.VRAM>>20)
			}
		}
	}
	t.Logf("%d processes have GPU data", n)
}

// amdFdinfo is a real-shaped amdgpu fdinfo file.
const amdFdinfo = `pos:	0
flags:	02100002
drm-driver:	amdgpu
drm-pdev:	0000:03:00.0
drm-client-id:	17
drm-memory-vram:	2097152 KiB
drm-memory-gtt:	8192 KiB
drm-engine-gfx:	%d ns
drm-engine-compute:	%d ns
drm-engine-capacity-gfx:	1
`

func writeFdinfo(t *testing.T, path string, gfx, compute uint64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf(amdFdinfo, gfx, compute)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseDRMFdinfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fdinfo")
	writeFdinfo(t, path, 5_000_000, 1_000)
	key, c, ok := parseDRMFdinfo(path)
	if !ok || key != "amdgpu/0000:03:00.0/17" || c.vram != 2<<30 || c.engine["gfx"] != 5_000_000 || c.engine["compute"] != 1_000 {
		t.Errorf("parse: %q %+v %v", key, c, ok)
	}
	if _, ok := c.engine["capacity-gfx"]; ok {
		t.Error("engine capacity is not a busy counter")
	}
}

func TestDRMUsage(t *testing.T) {
	prev := map[string]drmClient{"a": {pid: 10, engine: map[string]uint64{"gfx": 0, "compute": 0}}}
	cur := map[string]drmClient{
		"a": {pid: 10, engine: map[string]uint64{"gfx": 600_000_000, "compute": 100_000_000}, vram: 1 << 30},
		"b": {pid: 10, engine: map[string]uint64{"gfx": 1}, vram: 1 << 20}, // new client: memory only
		"c": {pid: 20, vram: 5 << 20},
	}
	got := drmUsage(prev, cur, time.Second)
	if u := got[10]; u.util != 60 || !u.utilKnown || u.vram != 1<<30+1<<20 {
		t.Errorf("pid 10: %+v", u)
	}
	if u := got[20]; u.utilKnown || u.vram != 5<<20 {
		t.Errorf("pid 20: %+v", u)
	}
}

func TestMergeUsage(t *testing.T) {
	got := mergeUsage(gpuUsage{1: {util: 30, utilKnown: true, vram: 100, vramKnown: true}},
		gpuUsage{1: {util: 50, utilKnown: true, vram: 20, vramKnown: true}, 2: {vram: 5, vramKnown: true}})
	if got[1].util != 50 || got[1].vram != 120 || got[2].vram != 5 || got[2].utilKnown {
		t.Errorf("merge: %+v", got)
	}
}

func TestReadDRMClientsFakeProc(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the /proc scan only runs on Linux (Windows reports link targets with backslashes)")
	}
	root := t.TempDir()
	mk := func(pid, fd, target string, gfx uint64) {
		fdDir := filepath.Join(root, pid, "fd")
		if err := os.MkdirAll(fdDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(fdDir, fd)); err != nil {
			t.Skipf("symlinks unavailable here: %v", err)
		}
		writeFdinfo(t, filepath.Join(root, pid, "fdinfo", fd), gfx, 0)
	}
	mk("4242", "7", "/dev/dri/renderD128", 10)
	mk("4242", "9", "/dev/dri/renderD128", 10)  // same client via a second fd: counted once
	mk("5151", "3", "/home/user/notes.txt", 10) // not a GPU file
	got := readDRMClients(root)
	if len(got) != 1 {
		t.Fatalf("want 1 client, got %d: %+v", len(got), got)
	}
	for _, c := range got {
		if c.pid != 4242 || c.vram != 2<<30 {
			t.Errorf("client: %+v", c)
		}
	}
}

func TestEnrichCompleteness(t *testing.T) {
	report := gpuUsage{1: {util: 50, utilKnown: true, vram: 9, vramKnown: true}}
	procs := func() []ProcInfo { return []ProcInfo{{PID: 1}, {PID: 2}} }

	complete := &gpuCollector{procs: procGPUReport{usage: report, complete: true}}
	ps := procs()
	complete.enrich(ps)
	if !ps[1].GPUKnown || ps[1].GPU != 0 || !ps[1].VRAMKnown {
		t.Errorf("complete source: unlisted process should read 0, got %+v", ps[1])
	}

	partial := &gpuCollector{procs: procGPUReport{usage: report}}
	ps = procs()
	partial.enrich(ps)
	if ps[1].GPUKnown || ps[1].VRAMKnown {
		t.Errorf("partial source: unlisted process should stay unknown, got %+v", ps[1])
	}
	if ps[0].GPU != 50 || ps[0].VRAM != 9 {
		t.Errorf("listed process: %+v", ps[0])
	}
}

func TestHasNonNVIDIAGPU(t *testing.T) {
	root := t.TempDir()
	vendor := func(card, id string) {
		p := filepath.Join(root, "class", "drm", card, "device", "vendor")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(id+"\n"), 0o644)
	}
	vendor("card0", "0x10de")
	if hasNonNVIDIAGPU(root) {
		t.Error("NVIDIA only")
	}
	vendor("card1", "0x8086")
	if !hasNonNVIDIAGPU(root) {
		t.Error("Intel iGPU present")
	}
}
