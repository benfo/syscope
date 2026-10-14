// Package tui is taskman's interactive full-screen dashboard.
package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/metrics"
	"taskman/internal/prefs"
)

const historyLen = 300

type Options struct {
	Sampler metrics.Sampler
	CPUInfo metrics.CPUInfo  // static processor info for the Performance tab
	Details detailsFetcher   // reads the details pane's per-process extras
	Now     func() time.Time // clock; nil means time.Now

	// Prefs are the starting settings (the preferences file with any flag
	// overrides); zero value means defaults. Save persists changes; nil means
	// don't save.
	Prefs prefs.Prefs
	Save  savePrefsFunc
	// Notice is shown in the footer at start (e.g. a preferences problem).
	Notice string
}

// Run blocks until the user quits.
func Run(ctx context.Context, o Options) error {
	p := tea.NewProgram(newModel(o), tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := p.Run()
	// A change made just before quitting may not have finished saving in the
	// background; save it synchronously.
	if m, ok := final.(model); ok && m.save != nil && m.settings() != m.saved {
		if serr := m.save(m.saved, m.settings()); serr != nil && err == nil {
			err = serr
		}
	}
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

// model is the Bubble Tea model: it owns sampling, the shared state both tabs
// read (latest snapshot, histories), and routes keys to the active tab.
type model struct {
	sampler metrics.Sampler
	refresh time.Duration
	cpuInfo metrics.CPUInfo

	snap      metrics.SystemSnapshot
	histories histories
	table     processTable

	tab        int
	perfKey    string // selected Performance device
	showHidden bool   // list hidden devices (e.g. idle virtual adapters)
	showHelp   bool
	helpOff    int          // scroll offset of the ? screen
	theme      int          // index into Themes
	picker     *themePicker // theme popover; nil when closed

	save  savePrefsFunc
	saved prefs.Prefs // settings as last saved (or loaded)

	status   string
	statusOK bool
	statusAt time.Time

	sampling bool
	seq      int // refresh chain; ticks from older chains are ignored
	width    int
	height   int
}

func newModel(o Options) model {
	s := o.Prefs
	if s == (prefs.Prefs{}) {
		s = prefs.Default()
	}
	m := model{
		sampler:   o.Sampler,
		cpuInfo:   o.CPUInfo,
		refresh:   s.Refresh.Duration,
		save:      o.Save,
		histories: histories{},
		table:     newProcessTable(o.Details, o.Now),
	}
	if m.refresh <= 0 {
		m.refresh = prefs.Default().Refresh.Duration
	}
	m.applySettings(s)
	m.saved = m.settings()
	if o.Notice != "" {
		m.setStatus(o.Notice, false)
	}
	return m
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
		m.table.setHeight(m.tableHeight())
		return m, nil

	case sampleMsg:
		m.sampling = false
		m.seq++
		if msg.err != nil {
			m.setStatus("sample failed: "+msg.err.Error(), false)
			return m, tick(m.refresh, m.seq)
		}
		m.snap = msg.snap
		m.recordHistory()
		m.table.setProcs(msg.procs, msg.snap)
		// The first sample only primes CPU deltas; follow up quickly.
		next := m.refresh
		if !m.snap.CPUKnown {
			next = min(next, 300*time.Millisecond)
		}
		return m, tea.Batch(tick(next, m.seq), m.table.detailsCmd())

	case tickMsg:
		if msg.seq != m.seq || m.sampling {
			return m, nil // stale tick from an earlier refresh chain
		}
		m.sampling = true
		return m, m.sampleCmd()

	case detailsMsg:
		m.table.setDetails(msg)
		return m, nil

	case killDoneMsg:
		m.showKillResult(msg)
		if m.sampling {
			return m, nil
		}
		m.sampling = true
		return m, m.sampleCmd()

	case tea.KeyMsg:
		next, cmd := m.handleKey(msg)
		return next.(model).persist(cmd)

	case prefsSavedMsg:
		if msg.err != nil {
			m.setStatus("couldn't save preferences: "+msg.err.Error(), false)
		}
		return m, nil
	}
	return m, nil
}

// handleKey routes a key: modal states first (kill prompt, search input,
// help), then global actions, then the active tab.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	var cmd tea.Cmd
	switch {
	case m.table.confirm != nil:
		var cancelled bool
		m.table, cmd, cancelled = m.table.confirmKey(key)
		if cancelled {
			m.setStatus("cancelled", true)
		}
		return m, cmd
	case m.table.searching:
		m.table, cmd = m.table.searchKey(msg)
		return m, cmd
	case m.picker != nil:
		return m.pickerKey(key), nil
	case m.showHelp:
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
	case actThemePicker:
		return m.openThemePicker(), nil
	}
	if m.tab == tabPerformance {
		return m.perfAction(act), nil
	}
	m.table, cmd = m.table.action(act)
	return m, cmd
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
