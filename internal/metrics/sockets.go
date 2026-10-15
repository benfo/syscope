package metrics

import (
	"context"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

// socketCollector lists the machine's TCP and UDP sockets (IPv4 and IPv6).
// Sockets are optional: on failure the list is left empty.
type socketCollector struct{}

func (socketCollector) collect(ctx context.Context, _ time.Time, snap *SystemSnapshot) error {
	conns, err := net.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return nil
	}
	snap.Sockets = make([]Socket, 0, len(conns))
	for _, c := range conns {
		snap.Sockets = append(snap.Sockets, toSocket(c))
	}
	return nil
}

func toSocket(c net.ConnectionStat) Socket {
	s := Socket{
		Proto:      "TCP",
		LocalIP:    c.Laddr.IP,
		LocalPort:  c.Laddr.Port,
		RemoteIP:   c.Raddr.IP,
		RemotePort: c.Raddr.Port,
		State:      c.Status,
		PID:        c.Pid,
	}
	if c.Type == syscall.SOCK_DGRAM {
		s.Proto, s.State = "UDP", "" // UDP has no connection state
	}
	if s.State == "NONE" {
		s.State = ""
	}
	return s
}
