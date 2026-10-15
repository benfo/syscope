package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/prefs"
	"github.com/benfo/syscope/internal/view"
)

// recorder is a fake savePrefsFunc that remembers each save.
type recorder struct{ saves [][2]prefs.Prefs }

func (r *recorder) save(before, after prefs.Prefs) error {
	r.saves = append(r.saves, [2]prefs.Prefs{before, after})
	return nil
}

func (r *recorder) last() prefs.Prefs { return r.saves[len(r.saves)-1][1] }

func modelWith(t *testing.T, start prefs.Prefs, rec *recorder) model {
	t.Cleanup(func() { applyTheme(DefaultTheme) })
	f := &fakeSampler{procs: goldenProcs()}
	m := newModel(Options{Sampler: f, Details: fakeDetails, Now: fakeNow, Prefs: start, Save: rec.save,
		CPUInfo: metrics.CPUInfo{Logical: 4}})
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return resample(t, m, f)
}

// press sends keys and runs any preference save they trigger.
func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.Update(keyMsg(k))
		m = next.(model)
		if cmd == nil {
			continue
		}
		msgs := []tea.Msg{cmd()}
		if batch, ok := msgs[0].(tea.BatchMsg); ok {
			msgs = nil
			for _, c := range batch {
				if c != nil {
					msgs = append(msgs, c())
				}
			}
		}
		for _, msg := range msgs {
			if saved, ok := msg.(prefsSavedMsg); ok {
				next, _ = m.Update(saved)
				m = next.(model)
			}
		}
	}
	return m
}

func TestSettingsApplyOnStart(t *testing.T) {
	start := prefs.Default()
	start.Theme, start.PerCoreCPU, start.DefaultLayout, start.DefaultSort = "nord", true, "tree", "vram"
	m := modelWith(t, start, &recorder{})
	tb := m.table
	if Themes[m.theme].Name != "nord" || !tb.perCoreCPU || tb.layout != view.LayoutTree ||
		tb.sortKey != view.SortVRAM || !tb.desc || m.tab != tabProcesses || tb.details.open {
		t.Errorf("settings not applied: theme %d %+v", m.theme, m.session())
	}
}

func TestSessionRestoredOnlyWhenEnabled(t *testing.T) {
	start := prefs.Default()
	start.Session = prefs.Session{Tab: "performance", Sort: "mem", Descending: true, Layout: "grouped", DetailsOpen: true, ShowHidden: true}
	m := modelWith(t, start, &recorder{})
	if m.tab != tabProcesses || m.table.details.open || m.table.layout != view.LayoutFlat {
		t.Error("a saved session must be ignored while restore is off")
	}
	start.RestoreSession = true
	m = modelWith(t, start, &recorder{})
	if m.tab != tabPerformance || !m.table.details.open || m.table.layout != view.LayoutGrouped ||
		m.table.sortKey != view.SortMem || !m.showHidden {
		t.Errorf("session not restored: %+v", m.session())
	}
	start.Session.Tab = "ports"
	if m = modelWith(t, start, &recorder{}); m.tab != tabPorts || m.session().Tab != "ports" {
		t.Errorf("the Ports tab should round-trip: tab %d", m.tab)
	}
}

func TestDashboardKeysDontSaveWithoutRestore(t *testing.T) {
	rec := &recorder{}
	m := modelWith(t, prefs.Default(), rec)
	press(t, m, "s", "z", "enter", "c", "2", ".")
	if len(rec.saves) != 0 {
		t.Errorf("dashboard keys are session-only: %d saves", len(rec.saves))
	}
}

func TestDashboardKeysSaveSessionWhenRestoring(t *testing.T) {
	rec := &recorder{}
	start := prefs.Default()
	start.RestoreSession = true
	m := modelWith(t, start, rec)
	press(t, m, "s")
	if len(rec.saves) == 0 {
		t.Fatal("with restore on, the session is saved")
	}
	got := rec.last()
	if got.Session.Sort != "mem" || got.DefaultSort != "cpu" {
		t.Errorf("only the session changes: %+v", got)
	}
}

func TestSettingsScreenSavesImmediately(t *testing.T) {
	rec := &recorder{}
	m := modelWith(t, prefs.Default(), rec)
	m = press(t, m, ",")
	if m.settingsUI == nil || !strings.Contains(m.View(), "Restore last session") {
		t.Fatal(", should open Settings")
	}
	m = press(t, m, "j", "l") // Refresh: 1s → 2s
	if m.refresh != 2*time.Second || len(rec.saves) != 1 || rec.last().Refresh.Duration != 2*time.Second {
		t.Errorf("refresh change should apply and save: %v, %d saves", m.refresh, len(rec.saves))
	}
	m = press(t, m, "j", "j", "l") // Default layout: flat → grouped, applied now too
	if m.table.layout != view.LayoutGrouped || rec.last().DefaultLayout != "grouped" {
		t.Errorf("default layout: %v / %q", m.table.layout, rec.last().DefaultLayout)
	}
	m = press(t, m, "G", "l") // Restore last session: on
	if !rec.last().RestoreSession {
		t.Error("restore setting should save")
	}
	m = press(t, m, "esc")
	if m.settingsUI != nil {
		t.Error("esc should close Settings")
	}
}

func TestThemeListFromSettings(t *testing.T) {
	rec := &recorder{}
	m := modelWith(t, prefs.Default(), rec)
	m = press(t, m, ",", "enter", "j", "j", "esc")
	if m.picker != nil || m.settingsUI == nil || m.theme != 0 || len(rec.saves) != 0 {
		t.Error("esc in the theme list reverts, saves nothing and returns to Settings")
	}
	m = press(t, m, "enter", "j", "enter")
	if m.picker != nil || m.settingsUI == nil || len(rec.saves) != 1 || rec.last().Theme != "dracula" {
		t.Errorf("enter keeps and saves the theme, back in Settings: %d saves", len(rec.saves))
	}
	m = press(t, m, "l") // h/l cycle the theme from the row itself
	if rec.last().Theme != "catppuccin-mocha" {
		t.Errorf("l on Theme should step to the next theme: %q", rec.last().Theme)
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
