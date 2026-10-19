// Package cli defines syscope's command-line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/prefs"
	"github.com/benfo/syscope/internal/tui"
)

// version is set at build time by scripts/release.sh (-ldflags -X). Without
// it, Go's module version is used: v0.x.y for `go install ...@v0.x.y`, a
// pseudo-version from git for a local build, else "dev".
var version = "dev"

func init() {
	if version != "dev" {
		return
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		version = bi.Main.Version
	}
}

// exitError carries a specific process exit code out of a command.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }

// Execute runs the CLI and returns the process exit code: 0 ok, 1 error,
// 2 partial failure.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := newRootCmd().ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "syscope:", err)
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 1
}

func newRootCmd() *cobra.Command {
	var refresh time.Duration
	var perCore bool
	var themeName, configPath string
	root := &cobra.Command{
		Use:     "syscope",
		Version: version,
		Short:   "Cross-platform process manager and performance monitor",
		Long: "Run without a command for the live dashboard, or use a command for scripting.\n" +
			"CPU% is normalized like Task Manager (100% = all cores busy) unless --per-core is set.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := cmd.Flags()
			if f.Changed("refresh") && refresh < minRefresh {
				return fmt.Errorf("--refresh must be at least %s", minRefresh)
			}
			if f.Changed("theme") {
				if _, err := themeFlag(themeName); err != nil {
					return err
				}
			}
			if !term.IsTerminal(os.Stdout.Fd()) {
				return errors.New("the dashboard needs a terminal; use `syscope ps` for scripting")
			}
			path, err := resolveConfigPath(configPath)
			if err != nil {
				return err
			}
			file, loadErr := prefs.Load(path)

			// Flags override the saved preferences for this run only.
			start := file
			if f.Changed("refresh") {
				start.Refresh = prefs.Duration{Duration: refresh}
			}
			if f.Changed("per-core") || f.Changed("irix") {
				start.PerCoreCPU = perCore
			}
			if f.Changed("theme") {
				start.Theme = themeName
			}
			if start.Refresh.Duration < minRefresh {
				start.Refresh.Duration = minRefresh
			}

			opts := tui.Options{
				Sampler: metrics.NewGopsutilSampler(),
				CPUInfo: metrics.ReadCPUInfo(cmd.Context()),
				Details: metrics.ReadProcDetails,
				Prefs:   start,
			}
			if loadErr != nil {
				// Don't save over a file the user may want to fix.
				opts.Notice = "preferences not loaded (" + loadErr.Error() + "); using defaults, not saving"
			} else {
				opts.Save = (&prefsSaver{path: path, file: file}).save
			}
			return tui.Run(cmd.Context(), opts)
		},
	}
	root.Flags().DurationVar(&refresh, "refresh", time.Second, "dashboard refresh interval (saved preference if not given)")
	addPerCoreFlag(root.Flags(), &perCore)
	root.Flags().StringVar(&themeName, "theme", "default", "colour theme (see `syscope themes`; , opens Settings)")
	root.PersistentFlags().StringVar(&configPath, "config", "", "preferences file (default: see `syscope config path`)")
	root.AddCommand(newPsCmd(), newKillCmd(), newThemesCmd(), newConfigCmd(&configPath))
	return root
}

// addPerCoreFlag registers --per-core, plus the old --irix name as a
// deprecated alias so existing scripts keep working.
func addPerCoreFlag(f *pflag.FlagSet, v *bool) {
	f.BoolVar(v, "per-core", false, "per-core CPU% (100% = one full core, like top)")
	f.BoolVar(v, "irix", false, "")
	_ = f.MarkDeprecated("irix", "use --per-core")
}
