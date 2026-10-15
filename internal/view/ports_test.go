package view

import (
	"testing"

	"github.com/benfo/syscope/internal/metrics"
)

func testSockets() []metrics.Socket {
	return []metrics.Socket{
		{Proto: "TCP", LocalIP: "0.0.0.0", LocalPort: 8080, State: "LISTEN", PID: 20},
		{Proto: "TCP", LocalIP: "127.0.0.1", LocalPort: 11434, State: "LISTEN", PID: 10},
		{Proto: "UDP", LocalIP: "::", LocalPort: 5353, PID: 30},
		{Proto: "TCP", LocalIP: "192.0.2.10", LocalPort: 50000, RemoteIP: "198.51.100.7", RemotePort: 443,
			State: "ESTABLISHED", PID: 20},
		{Proto: "TCP", LocalIP: "0.0.0.0", LocalPort: 445, State: "LISTEN"}, // owner unknown
	}
}

func testPortProcs() []metrics.ProcInfo {
	return []metrics.ProcInfo{{PID: 10, Name: "ollama.exe"}, {PID: 20, Name: "webserver"}, {PID: 30, Name: "mdns"}}
}

func ports(rows []PortRow) []uint32 {
	out := make([]uint32, len(rows))
	for i, r := range rows {
		out[i] = r.Port
	}
	return out
}

func TestPortRowsListeningSortedByPort(t *testing.T) {
	rows := PortRows(testSockets(), testPortProcs(), "", false, PortSortPort, false)
	if got := ports(rows); len(got) != 4 || got[0] != 445 || got[3] != 11434 {
		t.Errorf("listening ports: %v", got)
	}
	if rows[0].Process != "" || rows[3].Process != "ollama.exe" {
		t.Errorf("process names: %+v", rows)
	}
}

func TestPortRowsAllIncludesConnections(t *testing.T) {
	rows := PortRows(testSockets(), testPortProcs(), "", true, PortSortPort, false)
	if len(rows) != 5 {
		t.Fatalf("all: %d rows", len(rows))
	}
	var conn PortRow
	for _, r := range rows {
		if r.Port == 50000 {
			conn = r
		}
	}
	if conn.Remote != "198.51.100.7:443" || conn.State != "ESTABLISHED" {
		t.Errorf("connection row: %+v", conn)
	}
}

func TestPortRowsSearch(t *testing.T) {
	for q, want := range map[string]int{"80": 1, "ollama": 1, "127.0": 1, "20": 1, "zzz": 0} {
		if got := len(PortRows(testSockets(), testPortProcs(), q, false, PortSortPort, false)); got != want {
			t.Errorf("search %q: %d rows, want %d", q, got, want)
		}
	}
}

func TestPortRowsSortByProcess(t *testing.T) {
	rows := PortRows(testSockets(), testPortProcs(), "", false, PortSortProcess, false)
	if rows[0].Process != "" || rows[1].Process != "mdns" || rows[3].Process != "webserver" {
		t.Errorf("by process: %+v", rows)
	}
	if PortSortProcess.Next() != PortSortPort {
		t.Error("sort should cycle back to port")
	}
}

func TestHostPort(t *testing.T) {
	if got := HostPort("2001:db8::1", 443); got != "[2001:db8::1]:443" {
		t.Error(got)
	}
	if got := HostPort("192.0.2.1", 80); got != "192.0.2.1:80" {
		t.Error(got)
	}
}
