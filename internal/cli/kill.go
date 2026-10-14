package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/spf13/cobra"

	"taskman/internal/metrics"
	"taskman/internal/proc"
	"taskman/internal/view"
)

type killOptions struct {
	force bool
	name  string
	all   bool
}

func newKillCmd() *cobra.Command {
	var o killOptions
	cmd := &cobra.Command{
		Use:   "kill [pid...]",
		Short: "End processes by PID or name",
		Long: "End processes. Without --force this asks the process to exit (SIGTERM on Unix,\n" +
			"closing its windows on Windows); --force terminates it immediately.",
		Example: "  taskman kill 1234\n  taskman kill 1234 5678 --force\n  taskman kill --name notepad\n  taskman kill --name chrome --all --force",
		RunE: func(cmd *cobra.Command, args []string) error {
			pids, err := killTargets(cmd, args, o)
			if err != nil {
				return err
			}
			return runKill(cmd.OutOrStdout(), pids, o.force)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&o.force, "force", "f", false, "terminate immediately (SIGKILL / TerminateProcess)")
	f.StringVar(&o.name, "name", "", "kill processes with this executable name (\".exe\" optional)")
	f.BoolVar(&o.all, "all", false, "with --name, kill every match when more than one process matches")
	return cmd
}

func killTargets(cmd *cobra.Command, args []string, o killOptions) ([]int32, error) {
	switch {
	case o.name != "" && len(args) > 0:
		return nil, errors.New("give PIDs or --name, not both")
	case o.name == "" && len(args) == 0:
		return nil, errors.New("give at least one PID, or --name")
	case o.name == "":
		pids := make([]int32, 0, len(args))
		for _, a := range args {
			n, err := strconv.ParseInt(a, 10, 32)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid PID %q", a)
			}
			pids = append(pids, int32(n))
		}
		return pids, nil
	}

	_, procs, err := metrics.NewGopsutilSampler().Sample(cmd.Context())
	if err != nil {
		return nil, err
	}
	self := int32(os.Getpid())
	var matches []metrics.ProcInfo
	for _, p := range procs {
		if p.PID != self && view.MatchesName(p.Name, o.name) {
			matches = append(matches, p)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].PID < matches[j].PID })
	if len(matches) == 0 {
		return nil, fmt.Errorf("no process named %q", o.name)
	}
	if len(matches) > 1 && !o.all {
		w := cmd.ErrOrStderr()
		fmt.Fprintf(w, "%d processes match %q:\n", len(matches), o.name)
		const maxListed = 20
		for i, p := range matches {
			if i == maxListed {
				fmt.Fprintf(w, "  ... and %d more\n", len(matches)-maxListed)
				break
			}
			fmt.Fprintf(w, "  %d\t%s\t%s\n", p.PID, p.Name, view.ShortUser(p.User))
		}
		return nil, fmt.Errorf("pass --all to kill all %d, or give PIDs", len(matches))
	}
	pids := make([]int32, len(matches))
	for i, p := range matches {
		pids[i] = p.PID
	}
	return pids, nil
}

func runKill(w io.Writer, pids []int32, force bool) error {
	failed := 0
	for _, pid := range pids {
		if err := proc.Kill(pid, force); err != nil {
			failed++
			fmt.Fprintf(w, "PID %d: %v\n", pid, err)
			continue
		}
		if force {
			fmt.Fprintf(w, "PID %d: terminated\n", pid)
		} else {
			fmt.Fprintf(w, "PID %d: asked to exit\n", pid)
		}
	}
	switch {
	case failed == 0:
		return nil
	case failed == len(pids):
		return &exitError{code: 1, err: fmt.Errorf("failed to kill %d of %d", failed, len(pids))}
	}
	return &exitError{code: 2, err: fmt.Errorf("failed to kill %d of %d", failed, len(pids))}
}
