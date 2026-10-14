// Package cli defines taskman's command-line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"taskman/internal/metrics"
	"taskman/internal/tui"
)

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
	fmt.Fprintln(os.Stderr, "taskman:", err)
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 1
}

func newRootCmd() *cobra.Command {
	var refresh time.Duration
	var perCore bool
	root := &cobra.Command{
		Use:   "taskman",
		Short: "Cross-platform process manager and performance monitor",
		Long: "Run without a command for the live dashboard, or use a command for scripting.\n" +
			"CPU% is normalized like Task Manager (100% = all cores busy) unless --per-core is set.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !term.IsTerminal(os.Stdout.Fd()) {
				return errors.New("the dashboard needs a terminal; use `taskman ps` for scripting")
			}
			if refresh < 250*time.Millisecond {
				return errors.New("--refresh must be at least 250ms")
			}
			return tui.Run(cmd.Context(), tui.Options{
				Refresh:    refresh,
				PerCoreCPU: perCore,
				Sampler:    metrics.NewGopsutilSampler(),
				CPUInfo:    metrics.ReadCPUInfo(cmd.Context()),
				Details:    metrics.ReadProcDetails,
			})
		},
	}
	root.Flags().DurationVar(&refresh, "refresh", time.Second, "dashboard refresh interval")
	addPerCoreFlag(root.Flags(), &perCore)
	root.AddCommand(newPsCmd(), newKillCmd())
	return root
}

// addPerCoreFlag registers --per-core, plus the old --irix name as a
// deprecated alias so existing scripts keep working.
func addPerCoreFlag(f *pflag.FlagSet, v *bool) {
	f.BoolVar(v, "per-core", false, "per-core CPU% (100% = one full core, like top)")
	f.BoolVar(v, "irix", false, "")
	_ = f.MarkDeprecated("irix", "use --per-core")
}
