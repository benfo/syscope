package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/proc"
	"taskman/internal/view"
)

// pendingKill is a kill awaiting confirmation: y ends the target, t (offered
// when it started other processes) ends those too, any other key cancels.
type pendingKill struct {
	row   view.Row
	tree  []int32 // processes the target started, parents first
	force bool
}

func (pk pendingKill) prompt() string {
	verb := "End task"
	if pk.force {
		verb = "Force kill"
	}
	if len(pk.tree) == 0 {
		return fmt.Sprintf("%s %s? [y/N]", verb, rowDesc(pk.row))
	}
	return fmt.Sprintf("%s %s? [y/N · t: %s]", verb, rowDesc(pk.row), startedDesc(len(pk.tree)))
}

func startedDesc(n int) string {
	if n == 1 {
		return "and 1 process it started"
	}
	return fmt.Sprintf("and %d processes it started", n)
}

// killJob is a confirmed kill.
type killJob struct {
	desc  string
	pids  []int32 // in kill order
	force bool
}

// answer is the kill a prompt key confirms; false cancels. With the tree,
// parents go first so a parent can't start replacements for children
// already ended.
func (pk pendingKill) answer(key string) (killJob, bool) {
	job := killJob{desc: rowDesc(pk.row), pids: slices.Clone(pk.row.PIDs), force: pk.force}
	switch {
	case key == "y" || key == "Y":
		return job, true
	case (key == "t" || key == "T") && len(pk.tree) > 0:
		job.desc += " " + startedDesc(len(pk.tree))
		job.pids = append(job.pids, pk.tree...)
		return job, true
	}
	return killJob{}, false
}

// killDoneMsg reports a finished kill back to the model.
type killDoneMsg struct {
	desc   string
	force  bool
	failed int
	total  int
	err    error // last error
}

// killTarget is what d/D would end on the current tab: the selected process,
// or the owner of the selected socket.
func (m model) killTarget() (view.Row, bool) {
	switch m.tab {
	case tabProcesses:
		return m.table.selected()
	case tabPorts:
		return m.ports.owner()
	}
	return view.Row{}, false
}

// askKill opens the confirmation prompt for the current tab's selection.
func (m model) askKill(force bool) model {
	r, ok := m.killTarget()
	switch {
	case ok && r.Exited:
		m.setStatus(rowDesc(r)+" has already exited; only its parent can clear it from the list", false)
	case ok:
		m.confirm = &pendingKill{row: r, tree: m.running(view.Descendants(m.table.procs, r.PIDs)), force: force}
	default:
		if _, sel := m.ports.selected(); sel && m.tab == tabPorts {
			m.setStatus("which process owns this socket is unknown", false)
		}
	}
	return m
}

// running drops processes that have already exited (zombies) from pids.
func (m model) running(pids []int32) []int32 {
	exited := map[int32]bool{}
	for _, p := range m.table.procs {
		if p.Zombie() {
			exited[p.PID] = true
		}
	}
	return slices.DeleteFunc(pids, func(pid int32) bool { return exited[pid] })
}

// confirmKey answers the kill prompt.
func (m model) confirmKey(key string) (model, tea.Cmd) {
	job, ok := m.confirm.answer(key)
	m.confirm = nil
	if !ok {
		m.setStatus("cancelled", true)
		return m, nil
	}
	return m, killCmd(job)
}

func killCmd(job killJob) tea.Cmd {
	return func() tea.Msg {
		res := killDoneMsg{desc: job.desc, force: job.force, total: len(job.pids)}
		for _, pid := range job.pids {
			if err := proc.Kill(pid, job.force); err != nil {
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
