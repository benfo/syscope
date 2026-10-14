package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestExplainFollowsView(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	m = drive(t, m, key("?"))
	if v := m.View(); !strings.Contains(v, "About: Processes") || !strings.Contains(v, "Resident memory") {
		t.Error("Processes help should explain the columns")
	}

	m = drive(t, m, keyMsg("esc"), key("2"), key("?"))
	if v := strings.Join(m.helpContent(), "\n"); !strings.Contains(v, "About: CPU") || !strings.Contains(v, "P / E cores") {
		t.Error("CPU help should explain P/E cores on a hybrid CPU")
	}

	m = drive(t, m, keyMsg("esc"), key("j"), key("?"))
	v := strings.Join(m.helpContent(), "\n")
	if !strings.Contains(v, "About: Memory") || strings.Contains(v, "Committed") {
		t.Error("Memory help should not mention commit charge when the platform has none")
	}
	m.snap.CommitLimit = 1
	if !strings.Contains(strings.Join(m.helpContent(), "\n"), "Committed") {
		t.Error("Memory help should explain commit charge on Windows")
	}

	m = drive(t, m, keyMsg("esc"), key("j"), key("?"))
	if v := strings.Join(m.helpContent(), "\n"); !strings.Contains(v, "About: Disk 0 (C: D:)") || !strings.Contains(v, "Response") {
		t.Error("Disk help should explain active time and response")
	}

	m = drive(t, m, keyMsg("esc"))
	m.perfKey = "net:Wi-Fi"
	m = drive(t, m, key("?"))
	if v := strings.Join(m.helpContent(), "\n"); !strings.Contains(v, "About: Wi-Fi") || !strings.Contains(v, "12.5 MB/s") {
		t.Error("Network help should explain bits vs bytes")
	}
}

func TestHelpScrollsAndCloses(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	m = drive(t, m, tea.WindowSizeMsg{Width: 70, Height: 12}, key("?"))
	if !strings.Contains(m.renderFooter(), "lines 1–") {
		t.Fatalf("long help should show scroll position: %q", m.renderFooter())
	}
	m = drive(t, m, key("j"), key("j"))
	if m.helpOff != 2 {
		t.Errorf("j should scroll: offset %d", m.helpOff)
	}
	m = drive(t, m, key("G"))
	if want := len(m.helpContent()) - (m.height - chromeLines); m.helpOff != want {
		t.Errorf("G: offset %d, want %d", m.helpOff, want)
	}
	m = drive(t, m, key("x"))
	if !m.showHelp {
		t.Error("unbound keys should not close help")
	}
	for i, l := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(l); w > m.width {
			t.Errorf("line %d: width %d > %d", i, w, m.width)
		}
	}
	m = drive(t, m, keyMsg("esc"))
	if m.showHelp {
		t.Error("esc should close help")
	}
	m = drive(t, m, key("?"))
	if m.helpOff != 0 {
		t.Error("reopening help should start at the top")
	}
}
