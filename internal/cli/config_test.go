package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"taskman/internal/prefs"
)

func runRoot(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String()
}

func TestConfigCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if got := strings.TrimSpace(runRoot(t, "--config", path, "config", "path")); got != path {
		t.Errorf("path: %q", got)
	}
	if got := runRoot(t, "--config", path, "config", "show"); !strings.Contains(got, `theme = "default"`) {
		t.Errorf("show without a file should print defaults:\n%s", got)
	}
	saved := prefs.Default()
	saved.Theme = "nord"
	if err := prefs.Save(path, saved); err != nil {
		t.Fatal(err)
	}
	if got := runRoot(t, "--config", path, "config", "show"); !strings.Contains(got, `theme = "nord"`) {
		t.Errorf("show should print saved preferences:\n%s", got)
	}
	if got := runRoot(t, "--config", path, "config", "reset"); !strings.Contains(got, "removed") {
		t.Errorf("reset: %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("reset should delete the file")
	}
	if got := runRoot(t, "--config", path, "config", "reset"); !strings.Contains(got, "already apply") {
		t.Errorf("second reset: %q", got)
	}
}

func TestSaverKeepsFlagOverridesOutOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	file := prefs.Default()
	file.Theme = "nord"
	s := &prefsSaver{path: path, file: file}

	start := file
	start.Theme = "dracula" // --theme dracula for this run
	changed := start
	changed.DefaultLayout = "grouped" // the user changed it in Settings
	if err := s.save(start, changed); err != nil {
		t.Fatal(err)
	}
	got, err := prefs.Load(path)
	if err != nil || got.Theme != "nord" || got.DefaultLayout != "grouped" {
		t.Errorf("want saved theme kept and grouping saved, got %+v (%v)", got, err)
	}
}
