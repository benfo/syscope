package tui

import (
	"reflect"
	"strings"
	"taskman/internal/prefs"
	"testing"
)

// TestThemeDefinesEveryRole guards presets and user themes: an empty colour
// role would silently render in the terminal's default colour.
func TestThemeDefinesEveryRole(t *testing.T) {
	for _, th := range Themes {
		v := reflect.ValueOf(th)
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).String() == "" {
				t.Errorf("theme %q: %s is not set", th.Name, v.Type().Field(i).Name)
			}
		}
	}
}

func TestThemePickerPreviewKeepRevert(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultTheme) })
	m := sampled(t, &fakeSampler{})
	m = drive(t, m, key("T"))
	if m.picker == nil || !strings.Contains(m.View(), "j/k preview") {
		t.Fatal("T should open the theme picker over the screen")
	}
	before := ui.accent.GetForeground()
	m = drive(t, m, key("j"))
	if m.theme != 1 || ui.accent.GetForeground() == before {
		t.Error("moving should preview the theme live")
	}
	// Overshoot by one, then step back: one key, not a full cycle.
	m = drive(t, m, key("j"), key("k"), keyMsg("enter"))
	if m.picker != nil || m.theme != 1 || !strings.Contains(m.renderFooter(), "theme: dracula") {
		t.Errorf("enter should keep the previewed theme: theme %d", m.theme)
	}
	m = drive(t, m, key("T"), key("G"), keyMsg("esc"))
	if m.picker != nil || m.theme != 1 {
		t.Errorf("esc should revert to the theme before the picker opened: %d", m.theme)
	}
}

// TestPickerHonoursGlobalKeys: like help, the picker must not swallow global
// keys.
func TestPickerHonoursGlobalKeys(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultTheme) })
	m := sampled(t, &fakeSampler{})
	for _, b := range keymap {
		if b.scope != scopeGlobal {
			continue
		}
		for k := range b.keys {
			effect := false
			for _, tab := range []string{"1", "2"} {
				open := drive(t, m, key(tab), key("T"), key("j"), key("j")) // mid-list so up and down both move
				next, cmd := open.Update(keyMsg(k))
				after := next.(model)
				if cmd != nil || after.picker == nil || after.tab != open.tab || after.picker.sel != open.picker.sel {
					effect = true
				}
			}
			if !effect {
				t.Errorf("global key %q does nothing in the theme picker", k)
			}
		}
	}
}

func TestThemeByName(t *testing.T) {
	if th, ok := ThemeByName("Gruvbox-Dark"); !ok || th.Name != "gruvbox-dark" {
		t.Error("lookup should ignore case")
	}
	if _, ok := ThemeByName("nope"); ok {
		t.Error("unknown theme")
	}
	start := prefs.Default()
	start.Theme = Themes[3].Name
	m := newModel(Options{Prefs: start})
	t.Cleanup(func() { applyTheme(DefaultTheme) })
	if m.theme != 3 {
		t.Errorf("start theme: %d", m.theme)
	}
}
