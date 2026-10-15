package metrics

import (
	"context"
	stdnet "net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

func TestToSocket(t *testing.T) {
	tcp := toSocket(net.ConnectionStat{Type: syscall.SOCK_STREAM, Status: "LISTEN", Pid: 42,
		Laddr: net.Addr{IP: "0.0.0.0", Port: 8080}})
	if tcp.Proto != "TCP" || tcp.State != "LISTEN" || tcp.PID != 42 || tcp.LocalPort != 8080 || !tcp.Listening() {
		t.Errorf("tcp listener: %+v", tcp)
	}
	est := toSocket(net.ConnectionStat{Type: syscall.SOCK_STREAM, Status: "ESTABLISHED",
		Laddr: net.Addr{IP: "192.0.2.10", Port: 50000}, Raddr: net.Addr{IP: "198.51.100.7", Port: 443}})
	if est.Listening() || est.RemotePort != 443 {
		t.Errorf("established: %+v", est)
	}
	udp := toSocket(net.ConnectionStat{Type: syscall.SOCK_DGRAM, Status: "NONE", Laddr: net.Addr{IP: "::", Port: 5353}})
	if udp.Proto != "UDP" || udp.State != "" || !udp.Listening() {
		t.Errorf("udp: %+v", udp)
	}
}

func TestSocketCollectorFindsOwnListener(t *testing.T) {
	ln, err := stdnet.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skip("can't listen:", err)
	}
	defer ln.Close()
	port := uint32(ln.Addr().(*stdnet.TCPAddr).Port)

	var snap SystemSnapshot
	if err := (socketCollector{}).collect(context.Background(), time.Now(), &snap); err != nil {
		t.Fatal(err)
	}
	listening := 0
	found := false
	for _, s := range snap.Sockets {
		if s.Listening() {
			listening++
		}
		if s.LocalPort == port && s.Proto == "TCP" {
			found = s.Listening() && s.PID == int32(os.Getpid())
		}
	}
	t.Logf("%d sockets, %d listening", len(snap.Sockets), listening)
	if !found {
		t.Errorf("own listener on port %d not found with this PID", port)
	}
}
