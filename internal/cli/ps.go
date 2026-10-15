package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"taskman/internal/metrics"
	"taskman/internal/view"
)

type psOptions struct {
	sort     string
	reverse  bool
	top      int
	filter   string
	group    bool
	json     bool
	interval time.Duration
	perCore  bool
}

func newPsCmd() *cobra.Command {
	var o psOptions
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List processes with CPU and memory usage",
		Long: "List processes. CPU% is measured over --interval, so the command takes that long to run.\n" +
			"By default CPU% is normalized so 100% means every core is busy (like Task Manager);\n" +
			"--per-core shows it per core instead (100% = one full core, like top).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPs(cmd.Context(), cmd.OutOrStdout(), o)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.sort, "sort", "s", "cpu", "sort by cpu, mem, gpu, vram, pid or name")
	f.BoolVarP(&o.reverse, "reverse", "r", false, "reverse the sort direction")
	f.IntVarP(&o.top, "top", "n", 0, "show only the first N rows (0 = all)")
	f.StringVarP(&o.filter, "filter", "f", "", "only processes whose name contains, or PID starts with, this text")
	f.BoolVarP(&o.group, "group", "g", false, "group processes by executable name")
	f.BoolVar(&o.json, "json", false, "output JSON")
	f.DurationVarP(&o.interval, "interval", "i", time.Second, "CPU measurement window")
	addPerCoreFlag(f, &o.perCore)
	return cmd
}

func runPs(ctx context.Context, w io.Writer, o psOptions) error {
	key, err := view.ParseSortKey(o.sort)
	if err != nil {
		return err
	}
	if o.interval <= 0 {
		return errors.New("--interval must be positive")
	}

	s := metrics.NewGopsutilSampler()
	if _, _, err := s.Sample(ctx); err != nil {
		return err
	}
	select {
	case <-time.After(o.interval):
	case <-ctx.Done():
		return ctx.Err()
	}
	snap, procs, err := s.Sample(ctx)
	if err != nil {
		return err
	}

	rows := view.Rows(view.Filter(procs, o.filter), o.group)
	view.Sort(rows, key, key.DefaultDesc() != o.reverse)
	if o.top > 0 && len(rows) > o.top {
		rows = rows[:o.top]
	}
	if o.json {
		return writePsJSON(w, snap, rows, o)
	}
	return writePsTable(w, snap, rows, o)
}

func writePsTable(w io.Writer, snap metrics.SystemSnapshot, rows []view.Row, o psOptions) error {
	fmt.Fprintf(w, "CPU %.1f%%  MEM %s / %s  Procs %d  Up %s\n\n",
		snap.CPUTotal, view.FormatBytes(snap.MemUsed), view.FormatBytes(snap.MemTotal),
		snap.ProcCount, view.FormatUptime(snap.Uptime))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	first := "PID"
	if o.group {
		first = "COUNT"
	}
	hasGPU := len(snap.GPUs) > 0
	head := []string{first, "NAME", "USER", "CPU%", "MEM"}
	if hasGPU {
		head = append(head, "GPU%", "VRAM")
	}
	fmt.Fprintln(tw, strings.Join(head, "\t"))
	for _, r := range rows {
		id := strconv.Itoa(int(r.PID))
		if o.group {
			id = strconv.Itoa(r.Count)
		}
		cells := []string{id, r.Name + view.StateMark(r), view.ShortUser(r.User),
			view.FormatCPU(r.CPU, r.CPUKnown, snap.NumCPU, o.perCore), view.FormatMem(r.RSS, r.RSSKnown)}
		if hasGPU {
			cells = append(cells, view.FormatGPU(r.GPU, r.GPUKnown), view.FormatMem(r.VRAM, r.VRAMKnown))
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

type jsonSystem struct {
	CPUPercent        float64 `json:"cpu_percent"`
	NumCPU            int     `json:"num_cpu"`
	MemTotalBytes     uint64  `json:"mem_total_bytes"`
	MemUsedBytes      uint64  `json:"mem_used_bytes"`
	MemAvailableBytes uint64  `json:"mem_available_bytes"`
	SwapTotalBytes    uint64  `json:"swap_total_bytes"`
	SwapUsedBytes     uint64  `json:"swap_used_bytes"`
	ProcessCount      int     `json:"process_count"`
	UptimeSeconds     int64   `json:"uptime_seconds"`
}

// jsonRow uses pointers so unknown values (access denied) encode as null.
type jsonRow struct {
	PID        *int32   `json:"pid,omitempty"`
	Name       string   `json:"name"`
	Count      int      `json:"count,omitempty"`
	Exited     bool     `json:"exited,omitempty"` // zombie: exited, waiting for its parent
	Suspended  bool     `json:"suspended,omitempty"`
	PIDs       []int32  `json:"pids,omitempty"`
	User       string   `json:"user"`
	CPUPercent *float64 `json:"cpu_percent"`
	MemBytes   *uint64  `json:"mem_bytes"`
	// GPU fields are omitted when there's no data (no GPU, or unknown).
	GPUPercent *float64 `json:"gpu_percent,omitempty"`
	VRAMBytes  *uint64  `json:"vram_bytes,omitempty"`
}

// ifKnown returns a pointer to v, or nil (JSON null) when it isn't known.
func ifKnown[T any](v T, known bool) *T {
	if !known {
		return nil
	}
	return &v
}

func writePsJSON(w io.Writer, snap metrics.SystemSnapshot, rows []view.Row, o psOptions) error {
	mode := "normalized"
	if o.perCore {
		mode = "per-core"
	}
	out := struct {
		System  jsonSystem `json:"system"`
		CPUMode string     `json:"cpu_mode"`
		Grouped bool       `json:"grouped"`
		Rows    []jsonRow  `json:"processes"`
	}{
		System: jsonSystem{
			CPUPercent: snap.CPUTotal, NumCPU: snap.NumCPU,
			MemTotalBytes: snap.MemTotal, MemUsedBytes: snap.MemUsed, MemAvailableBytes: snap.MemAvailable,
			SwapTotalBytes: snap.SwapTotal, SwapUsedBytes: snap.SwapUsed,
			ProcessCount: snap.ProcCount, UptimeSeconds: int64(snap.Uptime.Seconds()),
		},
		CPUMode: mode,
		Grouped: o.group,
		Rows:    make([]jsonRow, 0, len(rows)),
	}
	for _, r := range rows {
		jr := jsonRow{Name: r.Name, User: r.User, Exited: r.Exited, Suspended: r.Suspended}
		if o.group {
			jr.Count, jr.PIDs = r.Count, r.PIDs
		} else {
			pid := r.PID
			jr.PID = &pid
		}
		jr.CPUPercent = ifKnown(view.CPUValue(r.CPU, snap.NumCPU, o.perCore), r.CPUKnown)
		jr.MemBytes = ifKnown(r.RSS, r.RSSKnown)
		jr.GPUPercent = ifKnown(r.GPU, r.GPUKnown)
		jr.VRAMBytes = ifKnown(r.VRAM, r.VRAMKnown)
		out.Rows = append(out.Rows, jr)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
