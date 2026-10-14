package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/prefs"
	"taskman/internal/view"
)

// savePrefsFunc persists a change: the settings when last saved (before) and
// now (after), so the caller can save only what changed.
type savePrefsFunc func(before, after prefs.Prefs) error

type prefsSavedMsg struct{ err error }

// settings captures the dashboard's current choices as preferences.
func (m model) settings() prefs.Prefs {
	t := m.table
	return prefs.Prefs{
		Theme:   Themes[m.theme].Name,
		Refresh: prefs.Duration{Duration: m.refresh},
		LastTab: strings.ToLower(tabNames[m.tab]),
		Processes: prefs.Processes{
			Sort: t.sortKey.String(), Descending: t.desc, Group: t.group,
			PerCoreCPU: t.perCoreCPU, DetailsOpen: t.details.open,
		},
		Performance: prefs.Performance{ShowHidden: m.showHidden},
	}
}

// applySettings sets the dashboard from preferences at startup. Unknown
// values fall back to defaults with a warning, rather than failing.
func (m *model) applySettings(s prefs.Prefs) {
	m.theme = 0
	if th, ok := ThemeByName(s.Theme); ok {
		for i := range Themes {
			if Themes[i].Name == th.Name {
				m.theme = i
			}
		}
	} else if s.Theme != "" {
		m.setStatus("unknown theme "+s.Theme+" in preferences; using default", false)
	}
	applyTheme(Themes[m.theme])

	for i, n := range tabNames {
		if strings.EqualFold(n, s.LastTab) {
			m.tab = i
		}
	}

	t := &m.table
	if k, err := view.ParseSortKey(s.Processes.Sort); err == nil {
		t.sortKey = k
	}
	t.desc, t.group = s.Processes.Descending, s.Processes.Group
	t.perCoreCPU, t.details.open = s.Processes.PerCoreCPU, s.Processes.DetailsOpen
	m.showHidden = s.Performance.ShowHidden
}

// persist saves the settings if they changed since the last save. The theme
// picker's live previews aren't saved; only the theme kept when it closes.
func (m model) persist(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	now := m.settings()
	if m.picker != nil || now == m.saved || m.save == nil {
		return m, cmd
	}
	before, save := m.saved, m.save
	m.saved = now
	return m, tea.Batch(cmd, func() tea.Msg { return prefsSavedMsg{save(before, now)} })
}
