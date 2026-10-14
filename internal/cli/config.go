package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"taskman/internal/prefs"
)

// minRefresh keeps the dashboard from sampling hard enough to load the
// machine it's measuring.
const minRefresh = 250 * time.Millisecond

// resolveConfigPath is --config when given, else the OS default location.
func resolveConfigPath(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	return prefs.DefaultPath()
}

// prefsSaver merges each change into the preferences as last saved, so only
// what the user changed is written (one-run flag overrides are not).
type prefsSaver struct {
	mu   sync.Mutex // saves run in the background and may overlap
	path string
	file prefs.Prefs
}

func (s *prefsSaver) save(before, after prefs.Prefs) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file = prefs.Merge(s.file, before, after)
	return prefs.Save(s.path, s.file)
}

func newConfigCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show, locate or reset saved preferences",
		Long: "taskman saves dashboard choices (theme, sort, grouping, …) automatically.\n" +
			"Command-line flags override them for one run without being saved.",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "Print where preferences are stored",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				path, err := resolveConfigPath(*configPath)
				if err == nil {
					fmt.Fprintln(cmd.OutOrStdout(), path)
				}
				return err
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "Print the preferences in effect (defaults where none are saved)",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				path, err := resolveConfigPath(*configPath)
				if err != nil {
					return err
				}
				p, err := prefs.Load(path)
				if err != nil {
					return err
				}
				return toml.NewEncoder(cmd.OutOrStdout()).Encode(p)
			},
		},
		&cobra.Command{
			Use:   "reset",
			Short: "Delete saved preferences so the defaults apply",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				path, err := resolveConfigPath(*configPath)
				if err != nil {
					return err
				}
				switch err := os.Remove(path); {
				case errors.Is(err, fs.ErrNotExist):
					fmt.Fprintln(cmd.OutOrStdout(), "no saved preferences; the defaults already apply")
				case err != nil:
					return err
				default:
					fmt.Fprintln(cmd.OutOrStdout(), "removed", path, "- the defaults apply from the next start")
				}
				return nil
			},
		},
	)
	return cmd
}
