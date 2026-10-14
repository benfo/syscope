package tui

import (
	"reflect"
	"testing"
)

// TestThemeDefinesEveryRole guards presets and user themes: an empty colour
// role would silently render in the terminal's default colour.
func TestThemeDefinesEveryRole(t *testing.T) {
	for _, th := range []Theme{DefaultTheme} {
		v := reflect.ValueOf(th)
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).String() == "" {
				t.Errorf("theme %q: %s is not set", th.Name, v.Type().Field(i).Name)
			}
		}
	}
}
