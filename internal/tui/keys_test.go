package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/metrics"
	"taskman/internal/view"
)

// TestKeymapConsistent enforces the key rules: a key means one thing per
// scope, tab keys never shadow global ones, and every binding is documented.
func TestKeymapConsistent(t *testing.T) {
	owner := map[scope]map[string]string{}
	for _, b := range keymap {
		if b.label == "" || b.desc == "" {
			t.Errorf("binding %+v needs a label and description", b)
		}
		if owner[b.scope] == nil {
			owner[b.scope] = map[string]string{}
		}
		for k := range b.keys {
			if prev, dup := owner[b.scope][k]; dup {
				t.Errorf("key %q bound twice in %s: %q and %q", k, scopeTitles[b.scope], prev, b.label)
			}
			owner[b.scope][k] = b.label
		}
	}
	for sc, keys := range owner {
		if sc == scopeGlobal {
			continue
		}
		for k, label := range keys {
			if g, ok := owner[scopeGlobal][k]; ok {
				t.Errorf("%s key %q (%s) shadows global %q", scopeTitles[sc], k, label, g)
			}
		}
	}
}

func TestEveryActionBound(t *testing.T) {
	bound := map[action]bool{}
	for _, b := range keymap {
		for _, a := range b.keys {
			bound[a] = true
		}
	}
	for a := actNone + 1; a < numActions; a++ {
		if !bound[a] {
			t.Errorf("action %d has no key", a)
		}
	}
}

func TestVimNavigation(t *testing.T) {
	var procs []metrics.ProcInfo
	for i := int32(1); i <= 40; i++ {
		procs = append(procs, p(i, "x", float64(100-i)))
	}
	m := sampled(t, &fakeSampler{procs: procs})
	m = drive(t, m, key("j"), key("j"))
	if m.table.scroll.cursor != 2 {
		t.Errorf("jj: cursor %d", m.table.scroll.cursor)
	}
	m = drive(t, m, key("k"))
	if m.table.scroll.cursor != 1 || m.confirm != nil {
		t.Errorf("k should move up, not kill: cursor %d", m.table.scroll.cursor)
	}
	m = drive(t, m, key("G"))
	if m.table.scroll.cursor != 39 {
		t.Errorf("G: cursor %d", m.table.scroll.cursor)
	}
	m = drive(t, m, key("g"))
	if m.table.scroll.cursor != 0 || (m.table.layout == view.LayoutGrouped) {
		t.Errorf("g should go to top without grouping: cursor %d group %v", m.table.scroll.cursor, (m.table.layout == view.LayoutGrouped))
	}
	m = drive(t, m, keyMsg("ctrl+d"))
	if want := m.table.rowsHeight() / 2; m.table.scroll.cursor != want {
		t.Errorf("ctrl+d: cursor %d, want %d", m.table.scroll.cursor, want)
	}
}

func TestHelpOverlay(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: []metrics.ProcInfo{p(1, "a", 1)}})
	m = drive(t, m, key("?"))
	v := strings.Join(m.helpContent(), "\n")
	if !m.showHelp || !strings.Contains(v, "end task / force kill") || !strings.Contains(v, "Everywhere") {
		t.Fatal("? should show Processes and global keys")
	}
	next, cmd := m.Update(key("q"))
	m = next.(model)
	if m.showHelp || cmd != nil {
		t.Error("q while help is open should only close help")
	}
	m = drive(t, m, key("2"), key("?"))
	if v := strings.Join(m.helpContent(), "\n"); !strings.Contains(v, "show hidden adapters") || strings.Contains(v, "end task") {
		t.Error("help should follow the current tab")
	}
}

func TestFooterFromKeymap(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	f := m.renderFooter()
	for _, want := range []string{"/", "search", "d/D", "kill", "z", "group", "?", "help"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer missing %q: %q", want, f)
		}
	}
}

// TestHelpHonoursGlobalKeys: overlays must not swallow global keys. Every
// global action changes something while help is open.
func TestHelpHonoursGlobalKeys(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultTheme) })
	m := sampled(t, &fakeSampler{})
	m = drive(t, m, tea.WindowSizeMsg{Width: 70, Height: 12})
	for _, b := range keymap {
		if b.scope != scopeGlobal {
			continue
		}
		for k, act := range b.keys {
			effect := false
			for _, tab := range []string{"1", "2"} { // "1" is a no-op on tab 1, so try both
				open := drive(t, m, key(tab), key("?"), key("j")) // scrolled, so up/top have an effect
				next, cmd := open.Update(keyMsg(k))
				after := next.(model)
				if cmd != nil || after.showHelp != open.showHelp || after.tab != open.tab || after.helpOff != open.helpOff || after.theme != open.theme {
					effect = true
				}
			}
			if !effect {
				t.Errorf("global key %q (action %d) does nothing in help", k, act)
			}
		}
	}
}

func TestTabInHelpSwitchesExplanation(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	m = drive(t, m, key("?"), keyMsg("tab"))
	if !m.showHelp || m.tab != tabPerformance || !strings.Contains(strings.Join(m.helpContent(), "\n"), "About: CPU") {
		t.Error("tab in help should switch tab and keep explaining")
	}
	m = drive(t, m, key("1"))
	if !m.showHelp || m.tab != tabProcesses {
		t.Error("1 in help should jump to Processes and keep help open")
	}
}

// Menus wrap; scrolling lists don't (see "Key rules" in ROADMAP.md).
func TestMenusWrapScrollingListsDont(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultTheme) })
	m := sampled(t, &fakeSampler{procs: goldenProcs()})

	s := drive(t, m, key(","), key("k"))
	if s.settingsUI.sel != len(settingRows)-1 {
		t.Errorf("k on the first setting should wrap to the last: %d", s.settingsUI.sel)
	}
	s = drive(t, s, key("j"))
	if s.settingsUI.sel != 0 {
		t.Errorf("j on the last setting should wrap to the first: %d", s.settingsUI.sel)
	}

	p := drive(t, m, key(","), keyMsg("enter"), key("k"))
	if p.picker.sel != len(Themes)-1 {
		t.Errorf("theme list should wrap: %d", p.picker.sel)
	}

	perf := drive(t, m, key("2"), key("k"))
	pages := perf.visiblePages()
	if sel, _ := perf.selectedPage(pages); sel.key() != pages[len(pages)-1].key() {
		t.Errorf("Performance devices should wrap to the last: %s", sel.key())
	}

	procs := drive(t, m, key("k"))
	if procs.table.scroll.cursor != 0 {
		t.Errorf("the process table must not wrap: cursor %d", procs.table.scroll.cursor)
	}
}
