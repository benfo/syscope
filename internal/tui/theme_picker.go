package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// themePicker is the theme list opened from Settings (Enter on Theme): moving
// previews each theme live on the real screen, Enter keeps it, Esc reverts to
// the starting theme. Settings stay open underneath.
type themePicker struct {
	sel  int // highlighted (and previewed) theme
	orig int // theme when the picker opened, restored on cancel
}

func (m model) openThemePicker() model {
	m.picker = &themePicker{sel: m.theme, orig: m.theme}
	return m
}

// pickerKey handles keys while the picker is open. Movement uses the global
// movement actions, so j/k/g/G mean the same as everywhere; tab keys still
// switch tabs, so a theme can be previewed on both.
func (m model) pickerKey(key string) model {
	p := *m.picker
	if key == "enter" {
		return m.closePicker(p.sel)
	}
	last := len(Themes) - 1
	switch act := lookup(scopeGlobal, key); act {
	case actDown, actHalfDown, actPageDown:
		p.sel = min(last, p.sel+1)
	case actUp, actHalfUp, actPageUp:
		p.sel = max(0, p.sel-1)
	case actTop:
		p.sel = 0
	case actBottom:
		p.sel = last
	case actSettings: // , : keep the previewed theme
		return m.closePicker(p.sel)
	case actCancel, actQuit:
		return m.closePicker(p.orig)
	case actHelp:
		m = m.closePicker(p.orig)
		m.showHelp, m.helpOff = true, 0
		return m
	case actTab1, actTab2, actNextTab, actPrevTab:
		m = m.switchTab(act)
	}
	m.picker = &p
	m.theme = p.sel
	applyTheme(Themes[p.sel])
	return m
}

func (m model) closePicker(theme int) model {
	m.picker = nil
	m.theme = theme
	applyTheme(Themes[theme])
	m.setStatus("theme: "+Themes[theme].Name, true)
	return m
}

// pickerBox renders the popover: one row per theme with a strip of its
// colours, so themes can be compared at a glance.
func (p themePicker) box() string {
	var b strings.Builder
	b.WriteString(ui.label.Render("Theme") + "\n")
	for i, t := range Themes {
		marker := "  "
		if i == p.sel {
			marker = ui.accent.Render("▸ ")
		}
		var chips strings.Builder
		for _, c := range []lipgloss.Color{t.Accent, t.Good, t.Warn, t.Bad, t.CPU, t.Memory, t.Disk, t.Network, t.GPU} {
			chips.WriteString(lipgloss.NewStyle().Foreground(c).Render("■"))
		}
		name := fit(t.Name, 18)
		if i == p.sel {
			name = ui.bold.Render(name)
		}
		b.WriteString(marker + name + " " + chips.String() + "\n")
	}
	b.WriteString(ui.muted.Render("j/k preview · enter keep · esc revert"))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Themes[p.sel].Accent).
		Padding(0, 1).Render(b.String())
}

// overlay draws box centred over the rendered screen base (w×h cells).
func overlay(base, box string, w, h int) string {
	lines := strings.Split(base, "\n")
	boxLines := strings.Split(box, "\n")
	bw := lipgloss.Width(box)
	top := max(0, (h-len(boxLines))/2)
	left := max(0, (w-bw)/2)
	for i, bl := range boxLines {
		y := top + i
		if y >= len(lines) {
			break
		}
		row := padANSI(lines[y], w)
		lines[y] = ansi.Truncate(row, left, "") + bl + ansi.TruncateLeft(row, left+bw, "")
	}
	return strings.Join(lines, "\n")
}
