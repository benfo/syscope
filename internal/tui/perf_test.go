package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestDevicePagesContract checks what every Performance page must provide,
// so a new device kind can't ship without help text or with a clashing key.
func TestDevicePagesContract(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	seen := map[string]bool{}
	for _, p := range m.pages(true) {
		k := p.key()
		if seen[k] {
			t.Errorf("duplicate page key %q", k)
		}
		seen[k] = true
		if p.name() == "" || p.helpTitle() == "" {
			t.Errorf("%s: missing name or help title", k)
		}
		if len(p.terms()) == 0 {
			t.Errorf("%s: no ? explanations", k)
		}
		if len(p.detail(80, 30)) == 0 {
			t.Errorf("%s: empty detail page", k)
		}
	}
	if len(seen) != 9 { // cpu, memory, disk + 2 volumes, Wi-Fi, hidden VPN, 2 GPUs
		t.Errorf("want 9 pages including hidden, got %d", len(seen))
	}
}

func TestVolumesNestedUnderTheirDisk(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 40}, key("2"))
	keys := []string{}
	for _, p := range m.visiblePages() {
		keys = append(keys, p.key())
	}
	if got := strings.Join(keys[2:5], " "); got != "disk:0 vol:0:C: vol:0:D:" {
		t.Errorf("volumes should follow their disk: %s", got)
	}
	m.perfKey = "vol:0:C:"
	v := m.View()
	for _, want := range []string{"  C:", "on Disk 0 (C: D:)", "Read · scale", "4.0 MB/s", "Active time and response time are on Disk 0"} {
		if !strings.Contains(v, want) {
			t.Errorf("volume page missing %q", want)
		}
	}
	m.perfKey = "vol:0:D:"
	if v := m.View(); !strings.Contains(v, "No per-volume transfer rates") || strings.Contains(v, "Read · scale") {
		t.Error("a volume without counters should say so, with no graphs")
	}
}

func TestSingleVolumeDiskHasNoSubEntry(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	m.snap.Disks[0].Volumes = m.snap.Disks[0].Volumes[:1]
	for _, p := range m.visiblePages() {
		if strings.HasPrefix(p.key(), "vol:") {
			t.Errorf("one volume would only repeat the disk: %s", p.key())
		}
	}
}
