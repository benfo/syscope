package prefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil || p != Default() {
		t.Errorf("got %+v, %v", p, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml") // directory created on save
	want := Default()
	want.Theme, want.Refresh.Duration, want.LastTab = "nord", 500*time.Millisecond, "performance"
	want.Processes = Processes{Sort: "vram", Descending: true, Layout: "tree", PerCoreCPU: true, DetailsOpen: true}
	want.Performance.ShowHidden = true
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got != want {
		t.Errorf("round trip: got %+v, %v", got, err)
	}
	b, _ := os.ReadFile(path)
	if s := string(b); !strings.HasPrefix(s, "# taskman preferences") || !strings.Contains(s, `refresh = "500ms"`) {
		t.Errorf("file should be readable TOML with a header:\n%s", s)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*")); len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("theme = \"dracula\"\n"), 0o644)
	p, err := Load(path)
	if err != nil || p.Theme != "dracula" || p.Refresh.Duration != time.Second || p.Processes.Sort != "cpu" {
		t.Errorf("got %+v, %v", p, err)
	}
}

func TestBadFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("theme = \n"), 0o644)
	p, err := Load(path)
	if err == nil || p != Default() {
		t.Errorf("want an error and defaults, got %+v, %v", p, err)
	}
	os.WriteFile(path, []byte("refresh = \"soon\"\n"), 0o644)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Errorf("bad duration: %v", err)
	}
}

func TestMergeKeepsOnlyChangedFields(t *testing.T) {
	file := Default()
	file.Theme = "nord"

	// This run started with --theme dracula and --per-core (not saved)...
	before := file
	before.Theme, before.Processes.PerCoreCPU = "dracula", true
	// ...and the user then changed the sort.
	after := before
	after.Processes.Sort = "mem"

	got := Merge(file, before, after)
	if got.Processes.Sort != "mem" {
		t.Error("the user's change should be saved")
	}
	if got.Theme != "nord" || got.Processes.PerCoreCPU {
		t.Errorf("flag overrides must not be saved: %+v", got)
	}
	after.Refresh.Duration = 2 * time.Second
	if Merge(file, before, after).Refresh.Duration != 2*time.Second {
		t.Error("duration changes merge too")
	}
}
