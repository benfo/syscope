// Package tui is taskman's interactive full-screen dashboard.
package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/metrics"
	"taskman/internal/proc"
	"taskman/internal/view"
)

const historyLen = 300

type Options struct {
	Refresh    time.Duration
	PerCoreCPU bool
	Sampler    metrics.Sampler
	CPUInfo    metrics.CPUInfo // static processor info for the Performance tab
}

// Run blocks until the user quits.
func Run(ctx context.Context, o Options) error {
	p := tea.NewProgram(newModel(o), tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	if errors.Is(err, tea.ErrProgramKilled) {
		return nil
	}
	return err
}

type sampleMsg struct {
	snap  metrics.SystemSnapshot
	procs []metrics.ProcInfo
	err   error
}

type tickMsg struct{ seq int }

type killDoneMsg struct {
	desc   string
	force  bool
	failed int
	total  int
	err    error // last error
}

// pendingKill is a kill awaiting y/N confirmation.
type pendingKill struct {
	row   view.Row
	force bool
}

type model struct {
	sampler metrics.Sampler
	refresh time.Duration
	cpuInfo metrics.CPUInfo

	tab         int
	perfKey     string // selected Performance device
	showAllNets bool
	showHelp    bool
	helpOff     int                 // scroll offset of the ? screen
	hist        map[string]*history // per-device histories, keyed "disk:0:read" etc.

	snap    metrics.SystemSnapshot
	procs   []metrics.ProcInfo
	rows    []view.Row
	cpuHist *history
	memHist *history

	sortKey    view.SortKey
	desc       bool
	group      bool
	perCoreCPU bool

	filter    textinput.Model
	filtering bool

	cursor int
	offset int
	selKey string

	confirm  *pendingKill
	status   string
	statusOK bool
	statusAt time.Time

	sampling bool
	seq      int
	width    int
	height   int
}

func newModel(o Options) model {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "name or PID"
	return model{
		sampler:    o.Sampler,
		cpuInfo:    o.CPUInfo,
		refresh:    o.Refresh,
		perCoreCPU: o.PerCoreCPU,
		sortKey:    view.SortCPU,
		desc:       true,
		filter:     ti,
		cpuHist:    newHistory(historyLen),
		hist:       map[string]*history{},
		memHist:    newHistory(historyLen),
	}
}

func (m model) Init() tea.Cmd {
	return m.sampleCmd()
}

func (m model) sampleCmd() tea.Cmd {
	s := m.sampler
	return func() tea.Msg {
		snap, procs, err := s.Sample(context.Background())
		return sampleMsg{snap, procs, err}
	}
}

func tick(d time.Duration, seq int) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{seq} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case sampleMsg:
		m.sampling = false
		m.seq++
		if msg.err != nil {
			m.setStatus("sample failed: "+msg.err.Error(), false)
			return m, tick(m.refresh, m.seq)
		}
		m.snap, m.procs = msg.snap, msg.procs
		if m.snap.CPUKnown {
			m.cpuHist.push(m.snap.CPUTotal)
		}
		if m.snap.MemTotal > 0 {
			m.memHist.push(float64(m.snap.MemUsed) / float64(m.snap.MemTotal) * 100)
		}
		m.recordHistory()
		m.rebuild()
		// The first sample only primes CPU deltas; follow up quickly.
		next := m.refresh
		if !m.snap.CPUKnown {
			next = min(next, 300*time.Millisecond)
		}
		return m, tick(next, m.seq)

	case tickMsg:
		if msg.seq != m.seq || m.sampling {
			return m, nil // stale tick from an earlier refresh chain
		}
		m.sampling = true
		return m, m.sampleCmd()

	case killDoneMsg:
		m.showKillResult(msg)
		if m.sampling {
			return m, nil
		}
		m.sampling = true
		return m, m.sampleCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.confirm != nil {
		pk := *m.confirm
		m.confirm = nil
		if key == "y" || key == "Y" {
			return m, killCmd(pk)
		}
		m.setStatus("cancelled", true)
		return m, nil
	}

	if m.filtering {
		switch key {
		case "enter":
			m.filtering = false
			m.filter.Blur()
		case "esc":
			m.filtering = false
			m.filter.Blur()
			m.filter.SetValue("")
		default:
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.rebuild()
			return m, cmd
		}
		m.rebuild()
		return m, nil
	}

	if m.showHelp {
		return m.helpAction(lookup(scopeGlobal, key)), nil
	}

	act := lookup(m.scope(), key)
	switch act {
	case actQuit:
		return m, tea.Quit
	case actHelp:
		m.showHelp, m.helpOff = true, 0
		return m, nil
	case actTab1, actTab2, actNextTab, actPrevTab:
		return m.switchTab(act), nil
	}
	if m.tab == tabPerformance {
		return m.perfAction(act), nil
	}
	return m.processAction(act)
}

func (m model) processAction(act action) (tea.Model, tea.Cmd) {
	half := max(1, m.bodyHeight()/2)
	switch act {
	case actDown:
		m.move(1)
	case actUp:
		m.move(-1)
	case actTop:
		m.move(-len(m.rows))
	case actBottom:
		m.move(len(m.rows))
	case actHalfDown:
		m.move(half)
	case actHalfUp:
		m.move(-half)
	case actPageDown:
		m.move(m.bodyHeight())
	case actPageUp:
		m.move(-m.bodyHeight())
	case actSortNext:
		m.sortKey = m.sortKey.Next()
		m.desc = m.sortKey.DefaultDesc()
		m.rebuild()
	case actSortReverse:
		m.desc = !m.desc
		m.rebuild()
	case actSearch:
		m.filtering = true
		return m, m.filter.Focus()
	case actCancel:
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.rebuild()
		}
	case actGroup:
		m.group = !m.group
		m.resetSelection()
		m.rebuild()
	case actCPUMode:
		m.perCoreCPU = !m.perCoreCPU
	case actEndTask, actForceKill:
		if r, ok := m.selected(); ok {
			m.confirm = &pendingKill{row: r, force: act == actForceKill}
		}
	}
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

func (m *model) setStatus(s string, ok bool) {
	m.status, m.statusOK, m.statusAt = s, ok, time.Now()
}

// rebuild derives the visible rows. Once the user has moved the cursor, the
// selection follows that process or group across refreshes; until then the
// cursor stays put so the top consumer stays highlighted.
func (m *model) rebuild() {
	m.rows = view.Rows(view.Filter(m.procs, m.filter.Value()), m.group)
	view.Sort(m.rows, m.sortKey, m.desc)

	found := false
	if m.selKey != "" {
		for i, r := range m.rows {
			if r.Key == m.selKey {
				m.cursor, found = i, true
				break
			}
		}
	}
	if !found {
		m.cursor = max(0, min(m.cursor, len(m.rows)-1))
		if m.selKey != "" {
			m.syncSelection() // selected process exited: follow its replacement row
		}
	}
	m.clampScroll()
}

func (m *model) move(delta int) {
	m.cursor = max(0, min(m.cursor+delta, len(m.rows)-1))
	m.syncSelection()
	m.clampScroll()
}

// resetSelection returns the cursor to the top and stops following a row.
func (m *model) resetSelection() {
	m.cursor, m.offset, m.selKey = 0, 0, ""
}

func (m *model) syncSelection() {
	if r, ok := m.selected(); ok {
		m.selKey = r.Key
	}
}

func (m *model) clampScroll() {
	h := m.bodyHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(0, min(m.offset, len(m.rows)-h))
}

func (m model) selected() (view.Row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return view.Row{}, false
	}
	return m.rows[m.cursor], true
}

func rowDesc(r view.Row) string {
	if r.Count > 1 {
		return fmt.Sprintf("%s (%d processes)", r.Name, r.Count)
	}
	return fmt.Sprintf("%s (PID %d)", r.Name, r.PID)
}

// switchTab handles the tab actions. It's shared by every mode that shows
// the tab bar, so tab keys work the same everywhere (including ? help).
func (m model) switchTab(act action) model {
	switch act {
	case actTab1, actTab2:
		m.tab = int(act - actTab1)
	case actNextTab:
		m.tab = (m.tab + 1) % numTabs
	case actPrevTab:
		m.tab = (m.tab + numTabs - 1) % numTabs
	}
	return m
}
