// Package view turns raw process samples into display rows: filtering,
// grouping, sorting and formatting. Everything here is pure.
package view

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"taskman/internal/metrics"
)

type SortKey int

const (
	SortCPU SortKey = iota
	SortMem
	SortGPU
	SortVRAM
	SortPID
	SortName
)

var sortNames = []string{"cpu", "mem", "gpu", "vram", "pid", "name"}

func (k SortKey) String() string { return sortNames[k] }

// Next cycles to the following sort column.
func (k SortKey) Next() SortKey { return (k + 1) % SortKey(len(sortNames)) }

// DefaultDesc reports the natural direction for a column: biggest consumers
// first for resources, ascending for identifiers.
func (k SortKey) DefaultDesc() bool { return k != SortPID && k != SortName }

// IsGPU reports whether the column only exists on machines with a GPU.
func (k SortKey) IsGPU() bool { return k == SortGPU || k == SortVRAM }

func ParseSortKey(s string) (SortKey, error) {
	for i, n := range sortNames {
		if strings.EqualFold(s, n) {
			return SortKey(i), nil
		}
	}
	return 0, fmt.Errorf("invalid sort key %q (want one of %s)", s, strings.Join(sortNames, ", "))
}

// Row is one line of the process table: a single process, or a group of
// processes sharing an executable name.
type Row struct {
	Key      string // stable identity for keeping selection across refreshes
	PID      int32  // first PID for groups
	PIDs     []int32
	Name     string
	User     string
	Count    int
	CPU      float64
	CPUKnown bool
	RSS      uint64
	RSSKnown bool

	GPU       float64
	GPUKnown  bool
	VRAM      uint64
	VRAMKnown bool

	// Nesting, for grouped and tree layouts (see Build).
	Depth      int    // 0 for top-level rows
	Parent     string // key of the row this one is nested under
	Expandable bool   // has children that can be shown
	Expanded   bool   // its children are shown below it
}

// Filter keeps processes whose name contains q (case-insensitive) or whose
// PID starts with q.
func Filter(procs []metrics.ProcInfo, q string) []metrics.ProcInfo {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return procs
	}
	var out []metrics.ProcInfo
	for _, p := range procs {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.HasPrefix(strconv.Itoa(int(p.PID)), q) {
			out = append(out, p)
		}
	}
	return out
}

// Rows converts processes to rows, optionally grouping by executable name.
func Rows(procs []metrics.ProcInfo, group bool) []Row {
	if !group {
		rows := make([]Row, len(procs))
		for i, p := range procs {
			rows[i] = Row{
				Key: ProcessKey(p.PID), PID: p.PID, PIDs: []int32{p.PID},
				Name: p.Name, User: p.User, Count: 1,
				CPU: p.CPU, CPUKnown: p.CPUKnown, RSS: p.RSS, RSSKnown: p.RSSKnown,
				GPU: p.GPU, GPUKnown: p.GPUKnown, VRAM: p.VRAM, VRAMKnown: p.VRAMKnown,
			}
		}
		return rows
	}

	idx := map[string]int{}
	var rows []Row
	for _, p := range procs {
		k := GroupName(p.Name)
		i, ok := idx[strings.ToLower(k)]
		if !ok {
			i = len(rows)
			idx[strings.ToLower(k)] = i
			rows = append(rows, Row{Key: "g:" + strings.ToLower(k), PID: p.PID, Name: k, User: p.User})
		}
		r := &rows[i]
		r.PIDs = append(r.PIDs, p.PID)
		r.Count++
		if r.User != p.User {
			r.User = "(multiple)"
		}
		if p.CPUKnown {
			r.CPU += p.CPU
			r.CPUKnown = true
		}
		if p.RSSKnown {
			r.RSS += p.RSS
			r.RSSKnown = true
		}
		if p.GPUKnown {
			r.GPU = min(r.GPU+p.GPU, 100)
			r.GPUKnown = true
		}
		if p.VRAMKnown {
			r.VRAM += p.VRAM
			r.VRAMKnown = true
		}
	}
	for i := range rows {
		sort.Slice(rows[i].PIDs, func(a, b int) bool { return rows[i].PIDs[a] < rows[i].PIDs[b] })
		rows[i].PID = rows[i].PIDs[0]
	}
	return rows
}

// GroupName is the display name for a group: the executable name without a
// Windows ".exe" suffix.
func GroupName(name string) string {
	if name == "" {
		return "(unknown)"
	}
	if strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name[:len(name)-4]
	}
	return name
}

// MatchesName reports whether a process name matches a user-supplied name,
// case-insensitively, with or without ".exe".
func MatchesName(procName, want string) bool {
	return strings.EqualFold(GroupName(procName), GroupName(want))
}

// Sort orders rows in place. Ties are broken by PID so the order is stable
// between refreshes.
func Sort(rows []Row, key SortKey, desc bool) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		var c int
		switch key {
		case SortCPU:
			c = cmpFloat(a.CPU, b.CPU)
		case SortMem:
			c = cmpUint(a.RSS, b.RSS)
		case SortGPU:
			c = cmpFloat(a.GPU, b.GPU)
		case SortVRAM:
			c = cmpUint(a.VRAM, b.VRAM)
		case SortName:
			c = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
		if c == 0 {
			c = cmpInt(a.PID, b.PID)
			if key != SortPID {
				return c < 0 // tiebreak always ascending
			}
		}
		if desc {
			return c > 0
		}
		return c < 0
	})
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpInt(a, b int32) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// CPUValue converts a normalized CPU% to the requested display mode.
func CPUValue(normalized float64, numCPU int, perCore bool) float64 {
	if perCore && numCPU > 0 {
		return normalized * float64(numCPU)
	}
	return normalized
}

// FormatCPU renders a CPU% cell, "--" when unknown.
func FormatCPU(normalized float64, known bool, numCPU int, perCore bool) string {
	if !known {
		return "--"
	}
	return strconv.FormatFloat(CPUValue(normalized, numCPU, perCore), 'f', 1, 64)
}

// FormatGPU renders a GPU% cell, "--" when unknown.
func FormatGPU(pct float64, known bool) string {
	if !known {
		return "--"
	}
	return strconv.FormatFloat(pct, 'f', 1, 64)
}

// FormatBytes renders a byte count with binary units, e.g. "1.2 GiB".
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTP"[exp])
}

// FormatMem renders a memory cell, "--" when unknown.
func FormatMem(b uint64, known bool) string {
	if !known {
		return "--"
	}
	return FormatBytes(b)
}

// FormatUptime renders a duration as e.g. "3d 4h 12m".
func FormatUptime(d time.Duration) string {
	d = d.Round(time.Minute)
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, h, m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// ShortUser drops a Windows "DOMAIN\" prefix from a user name.
func ShortUser(u string) string {
	if i := strings.LastIndexByte(u, '\\'); i >= 0 {
		return u[i+1:]
	}
	return u
}

// FormatByteRate renders bytes/second with decimal units, as Task Manager
// does for disks: "412 KB/s", "1.2 MB/s".
func FormatByteRate(bps float64) string {
	return decimal(bps, "B/s")
}

// FormatBitRate renders bytes/second as bits/second with decimal units, as
// Task Manager does for networks: "8.4 Mbps".
func FormatBitRate(bps float64) string {
	return decimal(bps*8, "bps")
}

func decimal(v float64, unit string) string {
	prefixes := []string{"", "K", "M", "G", "T"}
	i := 0
	for v >= 1000 && i < len(prefixes)-1 {
		v /= 1000
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", v, unit)
	}
	if v >= 100 {
		return fmt.Sprintf("%.0f %s%s", v, prefixes[i], unit)
	}
	return fmt.Sprintf("%.1f %s%s", v, prefixes[i], unit)
}

// Percent returns part as a percentage of total, or 0 when total is 0.
func Percent(part, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// FormatBytesCompact is FormatBytes without the space, for dense layouts:
// "1.2GiB".
func FormatBytesCompact(b uint64) string {
	return strings.Replace(FormatBytes(b), " ", "", 1)
}

// FormatUsedTotal renders used/total in one unit for the densest layouts:
// "12.3/31.7G", or "512/1024M" below a GiB.
func FormatUsedTotal(used, total uint64) string {
	const mib, gib = 1 << 20, 1 << 30
	if total < gib {
		return fmt.Sprintf("%.0f/%.0fM", float64(used)/mib, float64(total)/mib)
	}
	return fmt.Sprintf("%.1f/%.1fG", float64(used)/gib, float64(total)/gib)
}
