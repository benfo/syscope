package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/view"
)

// selectPort moves the Ports cursor to the row for port.
func selectPort(t *testing.T, m model, port uint32) model {
	t.Helper()
	for i, r := range m.ports.rows {
		if r.Port == port {
			m.ports.scroll.move(i-m.ports.scroll.cursor, m.ports.keys(), m.ports.bodyH)
			return m
		}
	}
	t.Fatalf("port %d not listed", port)
	return m
}

func TestPortsListsListenersThenAll(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("3"))
	if m.tab != tabPorts || m.scope() != scopePorts {
		t.Fatal("3 should open the Ports tab")
	}
	listening := len(m.ports.rows)
	if listening != 6 {
		t.Errorf("listening: %d rows", listening)
	}
	m = drive(t, m, key("."))
	if len(m.ports.rows) != len(goldenSockets()) {
		t.Errorf(". should list every socket: %d rows", len(m.ports.rows))
	}
}

func TestGoToProcessRevealsIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup []tea.Msg
	}{
		{"flat", nil},
		{"hidden by search", []tea.Msg{key("/"), key("S"), key("o"), key("n"), key("g"), keyMsg("enter")}},
		{"grouped", []tea.Msg{key("z")}},
		{"tree folded", []tea.Msg{key("t"), key("H")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := sampled(t, &fakeSampler{procs: goldenProcs()})
			m = drive(t, m, tc.setup...)
			m = drive(t, m, key("3"), key("."))
			m = selectPort(t, m, 52150) // a connection from the editor host
			m = drive(t, m, keyMsg("enter"))
			r, ok := m.table.selected()
			if m.tab != tabProcesses || !ok || r.PID != 55916 || r.Count != 1 || !m.table.details.open {
				t.Errorf("enter should select PID 55916 with details open: tab %d row %+v open %v",
					m.tab, r, m.table.details.open)
			}
		})
	}
}

func TestGoToProcessNestedInTree(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("t"), key("H"), key("3"), key("."))
	m = selectPort(t, m, 11434) // ollama.exe, a child of System
	m = drive(t, m, keyMsg("enter"))
	if r, _ := m.table.selected(); r.PID != 4120 || m.table.layout != view.LayoutTree {
		t.Errorf("should open System to show ollama.exe: %+v", r)
	}
}

func TestPortsUnknownOwner(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("3"))
	m = selectPort(t, m, 7680)
	for _, k := range []tea.KeyMsg{key("d"), keyMsg("enter")} {
		got := drive(t, m, k)
		if got.confirm != nil || got.tab != tabPorts || got.status == "" {
			t.Errorf("%q on a socket with no known owner should explain, not act", k.String())
		}
	}
	m = selectPort(t, m, 11434)
	if m = drive(t, m, key("D")); !killPrompt(t, m).force || killPrompt(t, m).row.PID != 4120 {
		t.Errorf("D should offer to force kill the owner: %+v", m.confirm)
	}
}

func TestPortsSearchIsSeparate(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("3"), key("/"), key("1"), key("1"), keyMsg("enter"))
	if len(m.ports.rows) != 1 || m.table.search.value() != "" {
		t.Errorf("ports search: %d rows; process search %q", len(m.ports.rows), m.table.search.value())
	}
	m = drive(t, m, keyMsg("esc"))
	if m.ports.search.value() != "" {
		t.Error("esc should clear the ports search")
	}
}
