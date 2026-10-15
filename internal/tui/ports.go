package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

// portsTable is the Ports tab: which process is listening on which port,
// and with show-all (.) every connection too.
type portsTable struct {
	socks []metrics.Socket
	procs []metrics.ProcInfo
	rows  []view.PortRow
	bodyH int

	sortKey view.PortSort
	desc    bool
	all     bool // show connections too, not just listening sockets

	search searchBox
	scroll scroller
}

func newPortsTable() portsTable {
	return portsTable{search: newSearchBox("port, name, PID or address")}
}

func (t *portsTable) setData(socks []metrics.Socket, procs []metrics.ProcInfo) {
	t.socks, t.procs = socks, procs
	t.rebuild()
}

func (t *portsTable) setHeight(h int) {
	t.bodyH = max(1, h)
	t.scroll.clamp(len(t.rows), t.bodyH)
}

func (t *portsTable) rebuild() {
	t.rows = view.PortRows(t.socks, t.procs, t.search.value(), t.all, t.sortKey, t.desc)
	t.scroll.refresh(t.keys(), t.bodyH)
}

func (t *portsTable) keys() rowKeys {
	return rowKeys{len(t.rows), func(i int) string { return t.rows[i].Key }}
}

func (t portsTable) selected() (view.PortRow, bool) {
	if c := t.scroll.cursor; c >= 0 && c < len(t.rows) {
		return t.rows[c], true
	}
	return view.PortRow{}, false
}

// owner is the selected socket's process as a killable row; false when
// nothing is selected or the owner is unknown.
func (t portsTable) owner() (view.Row, bool) {
	r, ok := t.selected()
	if !ok || r.PID == 0 {
		return view.Row{}, false
	}
	return view.Row{Key: view.ProcessKey(r.PID), PID: r.PID, PIDs: []int32{r.PID}, Name: processName(r), Count: 1}, true
}

func (t portsTable) apply(act action) (portsTable, tea.Cmd) {
	if delta, ok := scrollMove(act, len(t.rows), t.bodyH); ok {
		t.scroll.move(delta, t.keys(), t.bodyH)
		return t, nil
	}
	switch act {
	case actSortNext:
		t.sortKey, t.desc = t.sortKey.Next(), false
		t.rebuild()
	case actSortReverse:
		t.desc = !t.desc
		t.rebuild()
	case actSearch:
		return t, t.search.start()
	case actShowHidden:
		t.all = !t.all
		t.rebuild()
	case actCancel:
		if t.search.value() != "" {
			t.search.clear()
			t.rebuild()
		}
	}
	return t, nil
}

func (t portsTable) searchKey(msg tea.KeyMsg) (portsTable, tea.Cmd) {
	cmd := t.search.key(msg)
	t.rebuild()
	return t, cmd
}

// modeText summarises sort and what's listed for the tab bar.
func (t portsTable) modeText() string {
	shown := "listening"
	if t.all {
		shown = "all sockets"
	}
	return fmt.Sprintf("sort %s%s · %s ", t.sortKey, arrowFor(t.desc)[1:], shown)
}

func showAll(t portsTable) bool { return t.all }

func processName(r view.PortRow) string {
	switch {
	case r.PID == 0:
		return "(unknown)"
	case r.Process == "":
		return "(exited)"
	}
	return r.Process
}

var portColumns = []column[portsTable, view.PortRow]{
	{title: "PORT", width: 6, right: true, sortID: int(view.PortSortPort),
		cell: func(_ portsTable, r view.PortRow) string { return strconv.Itoa(int(r.Port)) }},
	{title: "PROTO", width: 6, sortID: int(view.PortSortProto),
		cell: func(_ portsTable, r view.PortRow) string { return r.Proto }},
	{title: "ADDRESS", sortID: noSort,
		cell: func(_ portsTable, r view.PortRow) string { return r.Address }},
	{title: "REMOTE", sortID: noSort, visible: showAll,
		cell: func(_ portsTable, r view.PortRow) string { return r.Remote }},
	{title: "STATE", width: 11, sortID: noSort, visible: showAll,
		cell: func(_ portsTable, r view.PortRow) string { return r.State }},
	{title: "PID", width: 7, right: true, sortID: int(view.PortSortPID),
		cell: func(_ portsTable, r view.PortRow) string {
			if r.PID == 0 {
				return ""
			}
			return strconv.Itoa(int(r.PID))
		}},
	{title: "PROCESS", sortID: int(view.PortSortProcess),
		cell: func(_ portsTable, r view.PortRow) string { return processName(r) }},
}

func (t portsTable) render(w int) string {
	return renderTable(tableView[portsTable, view.PortRow]{
		owner: t, cols: portColumns, rows: t.rows, scroll: t.scroll,
		height: t.bodyH, sortID: int(t.sortKey), desc: t.desc,
	}, w)
}

// terms are the ? explanations for the Ports tab.
func (t portsTable) terms() []term {
	out := []term{
		{"PORT", "The number other programs connect to. A port can only have one listener per address, " +
			"so \"address already in use\" means something here already has it: search for the port (/) to see what."},
		{"PROTO", "TCP for connections (web servers, databases, SSH). UDP for messages without a connection (DNS, discovery, games, video calls)."},
		{"ADDRESS", "Where it listens. 0.0.0.0 or :: means every network interface, so other machines can reach it unless a firewall blocks it. " +
			"127.0.0.1 or ::1 means this machine only."},
		{"PID / PROCESS", "The process that owns the socket. Enter goes to it on the Processes tab; d/D ends it. " +
			"(unknown) means the OS didn't say: on Linux, other users' sockets need root (sudo) to see their owner."},
		{"Listening", "Waiting for others to connect: TCP sockets in LISTEN, and UDP sockets bound to a port. Press . to show established connections too."},
	}
	if t.all {
		out = append(out,
			term{"REMOTE", "The other end of a connection: its address and port. Port 443 is usually HTTPS."},
			term{"STATE", "ESTABLISHED is an open connection. TIME_WAIT and CLOSE_WAIT are closing; many CLOSE_WAIT on one process can mean it isn't closing its connections."},
		)
	}
	return out
}
