package view

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"taskman/internal/metrics"
)

// PortSort is a Ports table sort column.
type PortSort int

const (
	PortSortPort PortSort = iota
	PortSortProto
	PortSortPID
	PortSortProcess
)

var portSortNames = []string{"port", "proto", "pid", "process"}

func (k PortSort) String() string { return portSortNames[k] }

// Next cycles through the sort columns.
func (k PortSort) Next() PortSort { return (k + 1) % PortSort(len(portSortNames)) }

// PortRow is one socket in the Ports table.
type PortRow struct {
	Key     string // stable identity for keeping selection across refreshes
	Proto   string
	Port    uint32
	Address string // local IP
	Remote  string // "IP:port"; "" when not connected
	State   string
	PID     int32 // 0 when unknown
	Process string
}

// PortRows lists the listening sockets, or every socket when all is set,
// filtered by search (port or PID prefix, process name or address) and
// sorted. Process names come from procs.
func PortRows(socks []metrics.Socket, procs []metrics.ProcInfo, search string, all bool, key PortSort, desc bool) []PortRow {
	names := make(map[int32]string, len(procs))
	for _, p := range procs {
		names[p.PID] = p.Name
	}
	q := strings.ToLower(strings.TrimSpace(search))
	var rows []PortRow
	for _, s := range socks {
		if !all && !s.Listening() {
			continue
		}
		r := PortRow{Proto: s.Proto, Port: s.LocalPort, Address: s.LocalIP, State: s.State, PID: s.PID}
		if s.PID != 0 {
			r.Process = names[s.PID]
		}
		if s.RemoteIP != "" && s.RemotePort != 0 {
			r.Remote = HostPort(s.RemoteIP, s.RemotePort)
		}
		r.Key = fmt.Sprintf("%s|%s|%d|%s|%d", r.Proto, r.Address, r.Port, r.Remote, r.PID)
		if q == "" || r.matches(q) {
			rows = append(rows, r)
		}
	}
	SortPorts(rows, key, desc)
	return rows
}

func (r PortRow) matches(q string) bool {
	return strings.HasPrefix(strconv.Itoa(int(r.Port)), q) ||
		(r.PID != 0 && strings.HasPrefix(strconv.Itoa(int(r.PID)), q)) ||
		strings.Contains(strings.ToLower(r.Process), q) ||
		strings.Contains(r.Address, q) || strings.Contains(r.Remote, q)
}

// SortPorts orders rows by key, then by port, protocol and address so equal
// rows keep a stable order between refreshes.
func SortPorts(rows []PortRow, key PortSort, desc bool) {
	slices.SortStableFunc(rows, func(a, b PortRow) int {
		var c int
		switch key {
		case PortSortProto:
			c = cmp.Compare(a.Proto, b.Proto)
		case PortSortPID:
			c = cmp.Compare(a.PID, b.PID)
		case PortSortProcess:
			c = cmp.Compare(strings.ToLower(a.Process), strings.ToLower(b.Process))
		}
		if c == 0 {
			c = cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.Proto, b.Proto),
				cmp.Compare(a.Address, b.Address), cmp.Compare(a.Remote, b.Remote), cmp.Compare(a.PID, b.PID))
		}
		if desc {
			return -c
		}
		return c
	})
}

// HostPort joins an IP and port, bracketing IPv6 addresses: "[::1]:443".
func HostPort(ip string, port uint32) string {
	if strings.Contains(ip, ":") {
		ip = "[" + ip + "]"
	}
	return ip + ":" + strconv.Itoa(int(port))
}
