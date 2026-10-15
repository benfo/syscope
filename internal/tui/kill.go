package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/benfo/syscope/internal/view"
)

// pendingKill is a kill awaiting confirmation: y ends the target, t (offered
// when it started other processes) ends those too, any other key cancels.
type pendingKill struct {
	row   view.Row
	tree  []int32 // processes the target started, parents first
	force bool
}

func (pk pendingKill) text() string {
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

func (pk pendingKill) respond(key string) tea.Cmd {
	if job, ok := pk.answer(key); ok {
		return runJob(job)
	}
	return nil
}

// answer is the kill a prompt key confirms; false cancels. With the tree,
// parents go first so a parent can't start replacements for children
// already ended.
func (pk pendingKill) answer(key string) (procJob, bool) {
	job := procJob{op: opEnd, desc: rowDesc(pk.row), pids: slices.Clone(pk.row.PIDs)}
	if pk.force {
		job.op = opForceEnd
	}
	switch {
	case key == "y" || key == "Y":
		return job, true
	case (key == "t" || key == "T") && len(pk.tree) > 0:
		job.desc += " " + startedDesc(len(pk.tree))
		job.pids = append(job.pids, pk.tree...)
		return job, true
	}
	return procJob{}, false
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
		m.confirm = pendingKill{row: r, tree: m.running(view.Descendants(m.table.procs, r.PIDs)), force: force}
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
