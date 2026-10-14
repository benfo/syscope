package tui

import (
	"strings"
	"testing"

	"taskman/internal/metrics"
	"taskman/internal/prefs"
	"taskman/internal/view"
)

// recorder is a fake savePrefsFunc that remembers each save.
type recorder struct{ saves [][2]prefs.Prefs }

func (r *recorder) save(before, after prefs.Prefs) error {
	r.saves = append(r.saves, [2]prefs.Prefs{before, after})
	return nil
}

func modelWith(t *testing.T, start prefs.Prefs, rec *recorder) model {
	t.Cleanup(func() { applyTheme(DefaultTheme) })
	f := &fakeSampler{procs: goldenProcs()}
	m := newModel(Options{Sampler: f, Details: fakeDetails, Now: fakeNow, Prefs: start, Save: rec.save,
		CPUInfo: metrics.CPUInfo{Logical: 4}})
	return resample(t, m, f)
}

// pressAndSave presses keys and runs any preference save they trigger.
func pressAndSave(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.Update(keyMsg(k))
		m = next.(model)
		if cmd != nil {
			if msg, ok := cmd().(prefsSavedMsg); ok {
				next, _ = m.Update(msg)
				m = next.(model)
			}
		}
	}
	return m
}

func TestSettingsRestoredOnStart(t *testing.T) {
	start := prefs.Default()
	start.Theme, start.LastTab = "nord", "performance"
	start.Processes = prefs.Processes{Sort: "vram", Descending: true, Group: true, PerCoreCPU: true, DetailsOpen: true}
	start.Performance.ShowHidden = true
	m := modelWith(t, start, &recorder{})
	tb := m.table
	if Themes[m.theme].Name != "nord" || m.tab != tabPerformance || tb.sortKey != view.SortVRAM || !tb.desc ||
		!tb.group || !tb.perCoreCPU || !tb.details.open || !m.showHidden {
		t.Errorf("settings not restored: %+v", m.settings())
	}
	if m.settings() != start {
		t.Errorf("settings should round-trip:\n got %+v\nwant %+v", m.settings(), start)
	}
}

func TestChangeIsSavedAsADiff(t *testing.T) {
	rec := &recorder{}
	m := modelWith(t, prefs.Default(), rec)
	m = pressAndSave(t, m, "s") // sort cpu → mem
	if len(rec.saves) != 1 {
		t.Fatalf("want 1 save, got %d", len(rec.saves))
	}
	before, after := rec.saves[0][0], rec.saves[0][1]
	if before.Processes.Sort != "cpu" || after.Processes.Sort != "mem" || before.Theme != after.Theme {
		t.Errorf("save should carry the change: %+v → %+v", before, after)
	}
	m = pressAndSave(t, m, "j", "k") // moving isn't a preference
	if len(rec.saves) != 1 {
		t.Errorf("navigation must not save: %d saves", len(rec.saves))
	}
}

func TestThemePickerSavesOnlyTheKeptTheme(t *testing.T) {
	rec := &recorder{}
	m := modelWith(t, prefs.Default(), rec)
	m = pressAndSave(t, m, "T", "j", "j", "esc")
	if len(rec.saves) != 0 {
		t.Errorf("previews and a reverted pick must not save: %d", len(rec.saves))
	}
	m = pressAndSave(t, m, "T", "j", "enter")
	if len(rec.saves) != 1 || rec.saves[0][1].Theme != "dracula" {
		t.Errorf("keeping a theme saves it once: %+v", rec.saves)
	}
}

func TestUnknownThemeFallsBack(t *testing.T) {
	start := prefs.Default()
	start.Theme = "neon-unicorn"
	m := modelWith(t, start, &recorder{})
	if m.theme != 0 || !strings.Contains(m.status, "unknown theme neon-unicorn") {
		t.Errorf("theme %d, status %q", m.theme, m.status)
	}
}
