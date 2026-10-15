package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/benfo/syscope/internal/prefs"
	"github.com/benfo/syscope/internal/view"
)

// savePrefsFunc persists a change: the preferences when last saved (before)
// and now (after), so the caller can save only what changed.
type savePrefsFunc func(before, after prefs.Prefs) error

type prefsSavedMsg struct{ err error }

// target is what should be on disk now: the settings (from the settings
// screen, plus any one-run flag overrides the user hasn't changed), and the
// current session only when "Restore last session" is on.
func (m model) target() prefs.Prefs {
	p := m.prefs
	p.Theme = Themes[m.theme].Name
	p.Refresh = prefs.Duration{Duration: m.refresh}
	if p.RestoreSession {
		p.Session = m.session()
	} else {
		p.Session = m.saved.Session // leave the last saved session alone
	}
	return p
}

// session captures what the user is doing now.
func (m model) session() prefs.Session {
	t := m.table
	return prefs.Session{
		Tab:  strings.ToLower(tabNames[m.tab]),
		Sort: t.sortKey.String(), Descending: t.desc, Layout: t.layout.String(),
		PerCoreCPU: t.perCoreCPU, DetailsOpen: t.details.open, ShowHidden: m.showHidden,
	}
}

// applyPrefs sets up the dashboard at startup: settings first, then the last
// session on top when restoring it. Unknown values fall back to defaults with
// a warning, rather than failing.
func (m *model) applyPrefs(p prefs.Prefs) {
	m.prefs = p
	m.refresh = p.Refresh.Duration
	if m.refresh <= 0 {
		m.refresh = prefs.Default().Refresh.Duration
	}
	m.theme = 0
	if i, ok := themeIndex(p.Theme); ok {
		m.theme = i
	} else if p.Theme != "" {
		m.setStatus("unknown theme "+p.Theme+" in preferences; using default", false)
	}
	applyTheme(Themes[m.theme])

	t := &m.table
	t.perCoreCPU = p.PerCoreCPU
	t.layout = parseLayout(p.DefaultLayout)
	t.sortKey = parseSort(p.DefaultSort)
	t.desc = t.sortKey.DefaultDesc()

	if s := p.Session; p.RestoreSession && s.Tab != "" {
		for i, n := range tabNames {
			if strings.EqualFold(n, s.Tab) {
				m.tab = i
			}
		}
		t.sortKey, t.desc, t.layout = parseSort(s.Sort), s.Descending, parseLayout(s.Layout)
		t.perCoreCPU, t.details.open, m.showHidden = s.PerCoreCPU, s.DetailsOpen, s.ShowHidden
	}
}

// persist saves when what should be on disk changed. Nothing is saved while
// the theme list previews themes; only the theme kept when it closes.
func (m model) persist(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	now := m.target()
	if m.picker != nil || now == m.saved || m.save == nil {
		return m, cmd
	}
	before, save := m.saved, m.save
	m.saved = now
	return m, tea.Batch(cmd, func() tea.Msg { return prefsSavedMsg{save(before, now)} })
}

func themeIndex(name string) (int, bool) {
	for i, t := range Themes {
		if strings.EqualFold(t.Name, name) {
			return i, true
		}
	}
	return 0, false
}

func parseLayout(s string) view.Layout {
	l, _ := view.ParseLayout(s) // flat when unknown
	return l
}

func parseSort(s string) view.SortKey {
	k, _ := view.ParseSortKey(s) // cpu when unknown
	return k
}
