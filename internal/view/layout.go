package view

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"taskman/internal/metrics"
)

// Layout is how the process list is arranged.
type Layout int

const (
	LayoutFlat    Layout = iota // one row per process
	LayoutGrouped               // one row per executable name, expandable
	LayoutTree                  // parent/child hierarchy, expandable
)

var layoutNames = []string{"flat", "grouped", "tree"}

func (l Layout) String() string { return layoutNames[l] }

func ParseLayout(s string) (Layout, error) {
	for i, n := range layoutNames {
		if strings.EqualFold(s, n) {
			return Layout(i), nil
		}
	}
	return LayoutFlat, fmt.Errorf("invalid layout %q (want one of %s)", s, strings.Join(layoutNames, ", "))
}

// Folds records which expandable rows are open: per-row choices on top of a
// default (groups start closed, the tree starts open).
type Folds struct {
	Default bool
	Open    map[string]bool
}

func (f Folds) IsOpen(key string) bool {
	if v, ok := f.Open[key]; ok {
		return v
	}
	return f.Default
}

// Build returns the visible rows for a layout: searched, sorted, and with the
// children of open rows inline (Depth > 0).
func Build(procs []metrics.ProcInfo, search string, layout Layout, key SortKey, desc bool, folds Folds) []Row {
	switch layout {
	case LayoutGrouped:
		return buildGrouped(Filter(procs, search), key, desc, folds)
	case LayoutTree:
		return buildTree(procs, search, key, desc, folds)
	}
	rows := Rows(Filter(procs, search), false)
	Sort(rows, key, desc)
	return rows
}

func buildGrouped(procs []metrics.ProcInfo, key SortKey, desc bool, folds Folds) []Row {
	members := map[string][]metrics.ProcInfo{}
	for _, p := range procs {
		k := strings.ToLower(GroupName(p.Name))
		members[k] = append(members[k], p)
	}
	groups := Rows(procs, true)
	Sort(groups, key, desc)

	var out []Row
	for _, g := range groups {
		g.Expandable = g.Count > 1
		g.Expanded = g.Expandable && folds.IsOpen(g.Key)
		out = append(out, g)
		if !g.Expanded {
			continue
		}
		kids := Rows(members[strings.TrimPrefix(g.Key, "g:")], false)
		Sort(kids, key, desc)
		for _, k := range kids {
			k.Depth, k.Parent = 1, g.Key
			out = append(out, k)
		}
	}
	return out
}

// buildTree nests processes under their parents. Processes whose parent isn't
// running (or is themselves) are roots. While searching, matches stay visible
// with their ancestors for context, and every node is open.
func buildTree(procs []metrics.ProcInfo, search string, key SortKey, desc bool, folds Folds) []Row {
	byPID := make(map[int32]metrics.ProcInfo, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	isChild := func(p metrics.ProcInfo) bool {
		_, ok := byPID[p.PPID]
		return ok && p.PPID != p.PID
	}

	visible := func(int32) bool { return true }
	if strings.TrimSpace(search) != "" {
		keep := map[int32]bool{}
		for _, m := range Filter(procs, search) {
			// Walk up to the root; the seen check guards against PPID cycles.
			for p, ok := m, true; ok && !keep[p.PID]; p, ok = byPID[p.PPID] {
				keep[p.PID] = true
				if !isChild(p) {
					break
				}
			}
		}
		visible = func(pid int32) bool { return keep[pid] }
		folds = Folds{Default: true} // show every match
	}

	children := map[int32][]metrics.ProcInfo{}
	var roots []metrics.ProcInfo
	for _, p := range procs {
		switch {
		case !visible(p.PID):
		case isChild(p):
			children[p.PPID] = append(children[p.PPID], p)
		default:
			roots = append(roots, p)
		}
	}

	var out []Row
	done := map[int32]bool{} // guards against PPID cycles
	var walk func(level []metrics.ProcInfo, depth int, parent string)
	walk = func(level []metrics.ProcInfo, depth int, parent string) {
		rows := Rows(level, false)
		Sort(rows, key, desc)
		for _, r := range rows {
			if done[r.PID] {
				continue
			}
			done[r.PID] = true
			kids := children[r.PID]
			r.Depth, r.Parent = depth, parent
			r.Expandable = len(kids) > 0
			r.Expanded = r.Expandable && folds.IsOpen(r.Key)
			out = append(out, r)
			if r.Expanded {
				walk(kids, depth+1, r.Key)
			}
		}
	}
	walk(roots, 0, "")
	// Processes in a parent cycle are reachable from no root; never hide
	// them, show them as roots. (Children of folded rows are reachable, just
	// not shown.)
	reachable := map[int32]bool{}
	for stack := roots; len(stack) > 0; {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !reachable[p.PID] {
			reachable[p.PID] = true
			stack = append(stack, children[p.PID]...)
		}
	}
	var unreached []metrics.ProcInfo
	for _, p := range procs {
		if visible(p.PID) && !reachable[p.PID] {
			unreached = append(unreached, p)
		}
	}
	walk(unreached, 0, "")
	return out
}

// ProcessKey is the row key of a single process, for selection and folding.
func ProcessKey(pid int32) string { return "p:" + strconv.Itoa(int(pid)) }

// groupKey is the row key of the group a process name belongs to.
func groupKey(name string) string { return "g:" + strings.ToLower(GroupName(name)) }

// Enclosing is the keys of the rows process pid is nested under in a
// layout, outermost first: they must be open for its row to show.
func Enclosing(procs []metrics.ProcInfo, layout Layout, pid int32) []string {
	byPID := make(map[int32]metrics.ProcInfo, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	p, ok := byPID[pid]
	switch {
	case !ok:
		return nil
	case layout == LayoutGrouped:
		return []string{groupKey(p.Name)}
	case layout != LayoutTree:
		return nil
	}
	var keys []string
	for _, a := range Ancestors(procs, pid) {
		keys = append(keys, ProcessKey(a))
	}
	slices.Reverse(keys)
	return keys
}

// Ancestors is the chain of running processes above pid: its parent, the
// parent's parent, and so on.
func Ancestors(procs []metrics.ProcInfo, pid int32) []int32 {
	byPID := make(map[int32]metrics.ProcInfo, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	var out []int32
	seen := map[int32]bool{pid: true} // guards against PPID cycles
	for p, ok := byPID[pid]; ok; {
		parent, found := byPID[p.PPID]
		if !found || seen[parent.PID] {
			break
		}
		seen[parent.PID] = true
		out = append(out, parent.PID)
		p = parent
	}
	return out
}

// Descendants is every process started, directly or indirectly, by the
// processes in roots (excluding roots), parents before children. A child
// must have started after its parent, so a PID reused by an unrelated
// process isn't mistaken for the parent.
func Descendants(procs []metrics.ProcInfo, roots []int32) []int32 {
	children := map[int32][]metrics.ProcInfo{}
	byPID := make(map[int32]metrics.ProcInfo, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
		if p.PPID != p.PID {
			children[p.PPID] = append(children[p.PPID], p)
		}
	}
	seen := map[int32]bool{}
	for _, pid := range roots {
		seen[pid] = true
	}
	var out []int32
	queue := slices.Clone(roots)
	for len(queue) > 0 {
		parent, ok := byPID[queue[0]]
		queue = queue[1:]
		if !ok {
			continue
		}
		for _, c := range children[parent.PID] {
			if seen[c.PID] || startedBefore(c, parent) {
				continue
			}
			seen[c.PID] = true
			out = append(out, c.PID)
			queue = append(queue, c.PID)
		}
	}
	return out
}

// startedBefore reports whether a started before b, when both times are known.
func startedBefore(a, b metrics.ProcInfo) bool {
	return !a.CreateTime.IsZero() && !b.CreateTime.IsZero() && a.CreateTime.Before(b.CreateTime)
}
