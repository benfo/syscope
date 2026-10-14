package tui

import (
	"strings"
	"testing"

	"taskman/internal/metrics"
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
	for a := actNone + 1; a <= actShowHidden; a++ {
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
	if m.cursor != 2 {
		t.Errorf("jj: cursor %d", m.cursor)
	}
	m = drive(t, m, key("k"))
	if m.cursor != 1 || m.confirm != nil {
		t.Errorf("k should move up, not kill: cursor %d", m.cursor)
	}
	m = drive(t, m, key("G"))
	if m.cursor != 39 {
		t.Errorf("G: cursor %d", m.cursor)
	}
	m = drive(t, m, key("g"))
	if m.cursor != 0 || m.group {
		t.Errorf("g should go to top without grouping: cursor %d group %v", m.cursor, m.group)
	}
	m = drive(t, m, keyMsg("ctrl+d"))
	if want := m.bodyHeight() / 2; m.cursor != want {
		t.Errorf("ctrl+d: cursor %d, want %d", m.cursor, want)
	}
}

func TestHelpOverlay(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: []metrics.ProcInfo{p(1, "a", 1)}})
	m = drive(t, m, key("?"))
	v := m.View()
	if !m.showHelp || !strings.Contains(v, "end task / force kill") || !strings.Contains(v, "Everywhere") {
		t.Fatal("? should show Processes and global keys")
	}
	next, cmd := m.Update(key("q"))
	m = next.(model)
	if m.showHelp || cmd != nil {
		t.Error("q while help is open should only close help")
	}
	m = drive(t, m, key("2"), key("?"))
	if v := m.View(); !strings.Contains(v, "show hidden adapters") || strings.Contains(v, "end task") {
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
