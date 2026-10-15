package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/benfo/syscope/internal/proc"
	"github.com/benfo/syscope/internal/view"
)

// prompt is a question in the footer that takes the next key (kill, suspend
// or quit confirmations). respond returns what the key confirms; nil cancels.
type prompt interface {
	text() string
	respond(key string) tea.Cmd
}

// confirmKey answers the open prompt.
func (m model) confirmKey(key string) (model, tea.Cmd) {
	cmd := m.confirm.respond(key)
	m.confirm = nil
	if cmd == nil {
		m.setStatus("cancelled", true)
	}
	return m, cmd
}

// procOp is something syscope does to processes.
type procOp int

const (
	opEnd procOp = iota
	opForceEnd
	opSuspend
	opResume
)

func (op procOp) apply(pid int32) error {
	switch op {
	case opForceEnd:
		return proc.Kill(pid, true)
	case opSuspend:
		return proc.Suspend(pid)
	case opResume:
		return proc.Resume(pid)
	}
	return proc.Kill(pid, false)
}

func (op procOp) pastTense() string {
	return [...]string{"asked to exit", "terminated", "suspended", "resumed"}[op]
}

// procJob is a confirmed action on processes.
type procJob struct {
	op   procOp
	desc string
	pids []int32 // in order
}

// procDoneMsg reports a finished job back to the model.
type procDoneMsg struct {
	job    procJob
	done   []int32 // PIDs it succeeded on
	failed int
	err    error // last error
}

func runJob(job procJob) tea.Cmd {
	return func() tea.Msg {
		res := procDoneMsg{job: job}
		for _, pid := range job.pids {
			if err := job.op.apply(pid); err != nil {
				res.failed++
				res.err = err
			} else {
				res.done = append(res.done, pid)
			}
		}
		return res
	}
}

func (m *model) showResult(r procDoneMsg) {
	desc, total := r.job.desc, len(r.job.pids)
	switch {
	case r.failed == 0:
		m.setStatus(fmt.Sprintf("%s: %s", desc, r.job.op.pastTense()), true)
	case r.failed == total:
		m.setStatus(fmt.Sprintf("%s: %v", desc, r.err), false)
	default:
		m.setStatus(fmt.Sprintf("%s: %d of %d failed: %v", desc, r.failed, total, r.err), false)
	}
}

func rowDesc(r view.Row) string {
	if r.Count > 1 {
		return fmt.Sprintf("%s (%d processes)", r.Name, r.Count)
	}
	return fmt.Sprintf("%s (PID %d)", r.Name, r.PID)
}
