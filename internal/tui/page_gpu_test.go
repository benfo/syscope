package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/metrics"
)

func fakeGPUs() []metrics.GPUStat {
	return []metrics.GPUStat{
		{ID: "0", Name: "NVIDIA GeForce RTX 3080 Ti Laptop GPU", Vendor: "NVIDIA",
			Util: 64, UtilKnown: true, MemUsed: 9 << 30, MemTotal: 12 << 30, MemKnown: true,
			SharedUsed: 1 << 30, SharedTotal: 32 << 30,
			Engines: []metrics.EngineStat{{Name: "3D", Util: 64}, {Name: "Copy", Util: 12}, {Name: "Video Decode", Util: 0}},
			TempC:   79, TempKnown: true, PowerW: 88.5, PowerLimitW: 95, PowerKnown: true,
			ClockMHz: 2400, ClockMaxMHz: 3090, MemClockMHz: 12000, ClockKnown: true, PState: 0,
			Throttle: []string{"power cap", "thermal (software)"}, ThrottleKnown: true},
		{ID: "1", Name: "Intel(R) Graphics", Vendor: "Intel", Util: 3, UtilKnown: true,
			MemUsed: 0, MemTotal: 128 << 20, MemKnown: true, SharedUsed: 700 << 20, SharedTotal: 32 << 30, PState: -1},
	}
}

func openGPUPage(t *testing.T, idx int) model {
	m := sampled(t, &fakeSampler{})
	m.perfKey = "gpu:" + fakeGPUs()[idx].ID
	return drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40}, key("2"))
}

func TestGPUPage(t *testing.T) {
	v := openGPUPage(t, 0).View()
	for _, want := range []string{"GPU 0 RTX 3080 Ti", "Dedicated memory · 9.0GiB / 12.0GiB", "79 °C",
		"88.5 / 95 W", "2400 / 3090 MHz", "P0", "power cap", "thermal (software)", "3D", "Copy"} {
		if !strings.Contains(v, want) {
			t.Errorf("GPU page missing %q", want)
		}
	}
}

func TestIntegratedGPUPageOmitsMissingSensors(t *testing.T) {
	m := openGPUPage(t, 1)
	v := m.View()
	for _, absent := range []string{"Temperature", "Power", "P-state", "Limited by"} {
		if strings.Contains(v, absent) {
			t.Errorf("iGPU page shows %q though the GPU doesn't report it", absent)
		}
	}
	help := strings.Join(drive(t, m, key("?")).helpContent(), "\n")
	if !strings.Contains(help, "Shared") || strings.Contains(help, "Temperature") {
		t.Error("iGPU help should explain only what's shown")
	}
}

func TestGPUHelp(t *testing.T) {
	help := strings.Join(drive(t, openGPUPage(t, 0), key("?")).helpContent(), "\n")
	for _, want := range []string{"About: GPU 0", "tokens per second", "Limited by", "P-state"} {
		if !strings.Contains(help, want) {
			t.Errorf("GPU help missing %q", want)
		}
	}
}

func TestStripGPU(t *testing.T) {
	s := sampled(t, &fakeSampler{}).stripGPU()
	if !strings.Contains(s, "64%") || !strings.Contains(s, "79°C") || !strings.Contains(s, "VRAM 9.0/12.0G") {
		t.Errorf("strip GPU: %q", s)
	}
}

func TestThrottleText(t *testing.T) {
	if s := throttleText(nil); !strings.Contains(s, "nothing") {
		t.Errorf("no reasons: %q", s)
	}
	if s := throttleText([]string{"idle"}); !strings.Contains(s, "idle") {
		t.Errorf("idle: %q", s)
	}
}

func TestShortGPUName(t *testing.T) {
	for in, want := range map[string]string{
		"NVIDIA GeForce RTX 3080 Ti Laptop GPU": "RTX 3080 Ti Laptop",
		"Intel(R) Graphics":                     "Intel Graphics",
		"AMD Radeon(TM) RX 7900 XTX":            "RX 7900 XTX",
	} {
		if got := shortGPUName(in); got != want {
			t.Errorf("shortGPUName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEngineGridHidesIdleNicheEngines(t *testing.T) {
	grid := strings.Join(engineGrid([]metrics.EngineStat{
		{Name: "3D", Util: 0}, {Name: "OFA", Util: 0}, {Name: "Security", Util: 4},
	}, 120), "\n")
	if !strings.Contains(grid, "3D") || strings.Contains(grid, "OFA") || !strings.Contains(grid, "Security") {
		t.Errorf("grid: %q", grid)
	}
}
