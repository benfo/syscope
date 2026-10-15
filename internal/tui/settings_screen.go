package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/benfo/syscope/internal/view"
)

// settingsScreen is the popover opened with ",": the one place where
// settings are changed and saved. Changes apply live.
type settingsScreen struct{ sel int }

// settingRow is one setting. Adding a setting means adding a row here and a
// field in prefs.Prefs.
type settingRow struct {
	label string
	value func(m model) string
	step  func(m model, dir int) model // h/l: previous/next value
	list  func(m model) model          // Enter: open a full list (optional)
}

var refreshChoices = []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second}

var settingRows = []settingRow{
	{
		label: "Theme",
		value: func(m model) string { return Themes[m.theme].Name },
		step: func(m model, dir int) model {
			m.theme = wrap(m.theme+dir, len(Themes))
			applyTheme(Themes[m.theme])
			return m
		},
		list: func(m model) model { return m.openThemePicker() },
	},
	{
		label: "Refresh",
		value: func(m model) string { return m.refresh.String() },
		step: func(m model, dir int) model {
			m.refresh = refreshChoices[clampIndex(indexOf(refreshChoices, m.refresh)+dir, len(refreshChoices))]
			return m
		},
	},
	{
		label: "CPU%",
		value: func(m model) string { return cpuModeName(m.prefs.PerCoreCPU) },
		step: func(m model, _ int) model {
			m.prefs.PerCoreCPU = !m.prefs.PerCoreCPU
			m.table.perCoreCPU = m.prefs.PerCoreCPU // apply now too
			return m
		},
	},
	{
		label: "Default layout",
		value: func(m model) string { return m.prefs.DefaultLayout },
		step: func(m model, dir int) model {
			l := view.Layout(wrap(int(parseLayout(m.prefs.DefaultLayout))+dir, 3))
			m.prefs.DefaultLayout = l.String()
			m.table.layout = l
			m.table.resetSelection()
			m.table.rebuild()
			return m
		},
	},
	{
		label: "Default sort",
		value: func(m model) string {
			k := parseSort(m.prefs.DefaultSort)
			return k.String() + arrowFor(k.DefaultDesc())
		},
		step: func(m model, dir int) model {
			k := view.SortKey(wrap(int(parseSort(m.prefs.DefaultSort))+dir, view.NumSortKeys))
			m.prefs.DefaultSort = k.String()
			m.table.sortKey, m.table.desc = k, k.DefaultDesc()
			m.table.rebuild()
			return m
		},
	},
	{
		label: "Restore last session",
		value: func(m model) string { return onOff(m.prefs.RestoreSession) },
		step: func(m model, _ int) model {
			m.prefs.RestoreSession = !m.prefs.RestoreSession
			return m
		},
	},
}

func (m model) openSettings() model {
	m.settingsUI = &settingsScreen{}
	return m
}

// settingsKey handles keys while settings are open. j/k/g/G move as
// everywhere; h/l (or ←/→) change the value; tab keys still switch tabs.
func (m model) settingsKey(key string) model {
	s := *m.settingsUI
	row := settingRows[s.sel]
	switch key {
	case "h", "left":
		return row.step(m, -1)
	case "l", "right":
		return row.step(m, +1)
	case "enter":
		if row.list != nil {
			return row.list(m)
		}
		return row.step(m, +1)
	}
	act := lookup(scopeGlobal, key)
	if sel, moved := menuMove(s.sel, len(settingRows), act); moved {
		s.sel = sel
		m.settingsUI = &s
		return m
	}
	switch act {
	case actCancel, actQuit, actSettings:
		m.settingsUI = nil
		return m
	case actHelp:
		m.settingsUI = nil
		m.showHelp, m.helpOff = true, 0
		return m
	case actTab1, actTab2, actNextTab, actPrevTab:
		m = m.switchTab(act)
	}
	m.settingsUI = &s
	return m
}

func (s settingsScreen) box(m model) string {
	const labelW, valueW = 22, 20
	var b strings.Builder
	b.WriteString(ui.label.Render("Settings") + ui.muted.Render("  saved as you change them") + "\n")
	for i, r := range settingRows {
		marker, label := "  ", fit(r.label, labelW)
		value := "‹ " + fit(r.value(m), valueW-4) + " ›"
		if r.list != nil {
			value = fit(r.value(m), valueW-2) + " ›"
		}
		if i == s.sel {
			marker, label, value = ui.accent.Render("▸ "), ui.bold.Render(label), ui.accent.Render(value)
		}
		b.WriteString(marker + label + value + "\n")
	}
	b.WriteString(ui.muted.Render("j/k select · h/l change · enter list · esc close"))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Themes[m.theme].Accent).
		Padding(0, 1).Render(b.String())
}

func cpuModeName(perCore bool) string {
	if perCore {
		return "per-core"
	}
	return "normalized"
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func arrowFor(desc bool) string {
	if desc {
		return " ▼"
	}
	return " ▲"
}

// clampIndex keeps i in [0, n) without cycling.
func clampIndex(i, n int) int { return max(0, min(i, n-1)) }

// indexOf finds d in ds, or the closest smaller choice.
func indexOf(ds []time.Duration, d time.Duration) int {
	best := 0
	for i, c := range ds {
		if c <= d {
			best = i
		}
	}
	return best
}
