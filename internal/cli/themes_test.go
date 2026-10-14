package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestThemesCommandListsAll(t *testing.T) {
	var out bytes.Buffer
	cmd := newThemesCmd()
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"default", "dracula", "catppuccin-mocha", "colorblind"} {
		if !strings.Contains(out.String(), name) {
			t.Errorf("missing %q", name)
		}
	}
}

func TestThemeFlagRejectsUnknown(t *testing.T) {
	if _, err := themeFlag("nope"); err == nil || !strings.Contains(err.Error(), "dracula") {
		t.Errorf("want an error listing the themes, got %v", err)
	}
}
