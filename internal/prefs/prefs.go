// Package prefs loads and saves the user's preferences: a TOML file in the
// OS config directory, so dashboard choices persist between sessions.
package prefs

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/BurntSushi/toml"
)

// Prefs is everything taskman remembers. Adding a preference means adding a
// field here; loading, saving and Merge handle it automatically.
type Prefs struct {
	Theme   string   `toml:"theme"`
	Refresh Duration `toml:"refresh"`
	LastTab string   `toml:"last_tab"` // "processes" or "performance"

	Processes   Processes   `toml:"processes"`
	Performance Performance `toml:"performance"`
}

type Processes struct {
	Sort        string `toml:"sort"` // cpu, mem, gpu, vram, pid, name
	Descending  bool   `toml:"descending"`
	Group       bool   `toml:"group"`
	PerCoreCPU  bool   `toml:"per_core_cpu"`
	DetailsOpen bool   `toml:"details_open"`
}

type Performance struct {
	ShowHidden bool `toml:"show_hidden"`
}

// Duration is a time.Duration written as text ("1s", "500ms").
type Duration struct{ time.Duration }

func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q", b)
	}
	d.Duration = v
	return nil
}

// Default is what a fresh install uses.
func Default() Prefs {
	return Prefs{
		Theme:     "default",
		Refresh:   Duration{time.Second},
		LastTab:   "processes",
		Processes: Processes{Sort: "cpu", Descending: true},
	}
}

// DefaultPath is config.toml in the OS's per-user config directory:
// %APPDATA%\taskman on Windows, ~/.config/taskman on Linux,
// ~/Library/Application Support/taskman on macOS.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "taskman", "config.toml"), nil
}

// Load reads preferences from path. A missing file gives the defaults, and
// fields missing from the file keep their defaults.
func Load(path string) (Prefs, error) {
	p := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if _, err := toml.Decode(string(b), &p); err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

const header = "# taskman preferences, saved automatically when you change them in the dashboard.\n" +
	"# Command-line flags override them for one run. Reset with: taskman config reset\n\n"

// Save writes preferences atomically: to a temporary file that then replaces
// path, so a crash can't leave a half-written file.
func Save(path string, p Prefs) error {
	var buf bytes.Buffer
	buf.WriteString(header)
	if err := toml.NewEncoder(&buf).Encode(p); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Merge returns base with every field that differs between before and after
// set to its after value. Saving Merge(fromFile, startOfChange, now) persists
// only what the user changed, so one-run flag overrides aren't saved.
func Merge(base, before, after Prefs) Prefs {
	merge(reflect.ValueOf(&base).Elem(), reflect.ValueOf(before), reflect.ValueOf(after))
	return base
}

func merge(base, before, after reflect.Value) {
	for i := 0; i < base.NumField(); i++ {
		b, a := before.Field(i), after.Field(i)
		if base.Field(i).Kind() == reflect.Struct && base.Field(i).Type() != reflect.TypeOf(Duration{}) {
			merge(base.Field(i), b, a)
			continue
		}
		if !reflect.DeepEqual(b.Interface(), a.Interface()) {
			base.Field(i).Set(a)
		}
	}
}
