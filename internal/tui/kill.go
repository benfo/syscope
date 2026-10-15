package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/proc"
	"taskman/internal/view"
)

// pendingKill is a kill awaiting y/N confirmation.
type pendingKill struct {
	row   view.Row
	force bool
}

// killDoneMsg reports a finished kill back to the model.
type killDoneMsg struct {
	desc   string
	force  bool
	failed int
	total  int
	err    error // last error
}

// killTarget is what d/D would end on the current tab.
func (m model) killTarget() (view.Row, bool) {
	if m.tab == tabProcesses {
		return m.table.selected()
	}
	return view.Row{}, false
}

// askKill opens the y/N prompt for the current tab's selection.
func (m model) askKill(force bool) model {
	if r, ok := m.killTarget(); ok {
		m.confirm = &pendingKill{row: r, force: force}
	}
	return m
}

// confirmKey answers the y/N kill prompt; any key but y cancels.
func (m model) confirmKey(key string) (model, tea.Cmd) {
	pk := *m.confirm
	m.confirm = nil
	if key == "y" || key == "Y" {
		return m, killCmd(pk)
	}
	m.setStatus("cancelled", true)
	return m, nil
}

func killCmd(pk pendingKill) tea.Cmd {
	return func() tea.Msg {
		res := killDoneMsg{desc: rowDesc(pk.row), force: pk.force, total: len(pk.row.PIDs)}
		for _, pid := range pk.row.PIDs {
			if err := proc.Kill(pid, pk.force); err != nil {
				res.failed++
				res.err = err
			}
		}
		return res
	}
}

func rowDesc(r view.Row) string {
	if r.Count > 1 {
		return fmt.Sprintf("%s (%d processes)", r.Name, r.Count)
	}
	return fmt.Sprintf("%s (PID %d)", r.Name, r.PID)
}

func (m *model) showKillResult(r killDoneMsg) {
	verb := "asked to exit"
	if r.force {
		verb = "terminated"
	}
	switch {
	case r.failed == 0:
		m.setStatus(fmt.Sprintf("%s: %s", r.desc, verb), true)
	case r.failed == r.total:
		m.setStatus(fmt.Sprintf("%s: %v", r.desc, r.err), false)
	default:
		m.setStatus(fmt.Sprintf("%s: %d of %d failed: %v", r.desc, r.failed, r.total, r.err), false)
	}
}
