package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

// suspendPrompt asks before suspending: a paused process can be one you need
// (suspending the window manager freezes the screen). Resuming never asks.
type suspendPrompt struct{ row view.Row }

func (sp suspendPrompt) text() string { return fmt.Sprintf("Suspend %s? [y/N]", rowDesc(sp.row)) }

func (sp suspendPrompt) respond(key string) tea.Cmd {
	if key != "y" && key != "Y" {
		return nil
	}
	return runJob(procJob{op: opSuspend, desc: rowDesc(sp.row), pids: slices.Clone(sp.row.PIDs)})
}

// toggleSuspend resumes the selected process if it's suspended, otherwise
// asks to suspend it.
func (m model) toggleSuspend() (model, tea.Cmd) {
	r, ok := m.table.selected()
	switch {
	case !ok:
	case r.Exited:
		m.setStatus(rowDesc(r)+" has already exited", false)
	case r.Suspended:
		return m, runJob(procJob{op: opResume, desc: rowDesc(r), pids: slices.Clone(r.PIDs)})
	case m.runsSelf(r):
		m.setStatus("can't suspend syscope or the terminal it runs in: nothing could resume it", false)
	default:
		m.confirm = suspendPrompt{r}
	}
	return m, nil
}

// runsSelf reports whether r includes syscope or one of its ancestors
// (the shell and terminal it runs in).
func (m model) runsSelf(r view.Row) bool {
	chain := append(view.Ancestors(m.table.procs, m.selfPID), m.selfPID)
	return slices.ContainsFunc(r.PIDs, func(pid int32) bool { return slices.Contains(chain, pid) })
}

// trackSuspended remembers processes this session suspended, so quitting
// can offer to resume them. Start times guard against reused PIDs.
func (m *model) trackSuspended(r procDoneMsg) {
	switch r.job.op {
	case opSuspend:
		for _, pid := range r.done {
			if p, ok := m.procByPID(pid); ok {
				m.suspended[pid] = p.CreateTime
			}
		}
	case opResume:
		for _, pid := range r.done {
			delete(m.suspended, pid)
		}
	}
}

// stillSuspended is what this session suspended that is still suspended.
func (m model) stillSuspended() []int32 {
	var out []int32
	for pid, started := range m.suspended {
		if p, ok := m.procByPID(pid); ok && p.CreateTime.Equal(started) && p.Suspended() {
			out = append(out, pid)
		}
	}
	slices.Sort(out)
	return out
}

func (m model) procByPID(pid int32) (p metrics.ProcInfo, ok bool) {
	for _, p := range m.table.procs {
		if p.PID == pid {
			return p, true
		}
	}
	return p, false
}

// quit asks to resume processes this session left suspended; otherwise it
// quits straight away.
func (m model) quit() (model, tea.Cmd) {
	if pids := m.stillSuspended(); len(pids) > 0 {
		m.confirm = quitPrompt{pids}
		return m, nil
	}
	return m, tea.Quit
}

// quitPrompt: Enter or y resumes and quits, n quits leaving them suspended,
// anything else stays.
type quitPrompt struct{ pids []int32 }

func (qp quitPrompt) text() string {
	what := "the process"
	if n := len(qp.pids); n > 1 {
		what = fmt.Sprintf("the %d processes", n)
	}
	return fmt.Sprintf("Resume %s you suspended before quitting? [Y/n]", what)
}

func (qp quitPrompt) respond(key string) tea.Cmd {
	switch key {
	case "y", "Y", "enter":
		return tea.Sequence(runJob(qp.job()), tea.Quit)
	case "n", "N":
		return tea.Quit
	}
	return nil
}

func (qp quitPrompt) job() procJob {
	return procJob{op: opResume, desc: "suspended processes", pids: qp.pids}
}
