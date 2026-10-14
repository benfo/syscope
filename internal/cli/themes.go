package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"taskman/internal/tui"
)

func newThemesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "themes",
		Short: "List colour themes with a preview",
		Long: "List the built-in colour themes. Use one with `taskman --theme <name>` for one run, or\n" +
			"press , in the dashboard for Settings, where themes preview live and are saved.\n" +
			"Themes colour text only; they look best with a matching terminal background.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, t := range tui.Themes {
				fmt.Fprintln(cmd.OutOrStdout(), tui.Swatch(t))
			}
			return nil
		},
	}
}

// themeFlag resolves --theme to a built-in theme.
func themeFlag(name string) (tui.Theme, error) {
	t, ok := tui.ThemeByName(name)
	if !ok {
		return tui.Theme{}, fmt.Errorf("unknown theme %q (choose from: %s)", name, strings.Join(tui.ThemeNames(), ", "))
	}
	return t, nil
}
