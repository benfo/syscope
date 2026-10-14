package metrics

import (
	"context"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestDefaultVisible(t *testing.T) {
	cases := []struct {
		up, loop, phys, seen, want bool
	}{
		{true, false, true, false, true},   // Wi-Fi, idle
		{true, false, false, true, true},   // VPN with traffic
		{true, false, false, false, false}, // idle Hyper-V switch
		{false, false, true, true, false},  // unplugged Ethernet
		{true, true, false, true, false},   // loopback
	}
	for _, c := range cases {
		if got := defaultVisible(c.up, c.loop, c.phys, c.seen); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestPickAddrs(t *testing.T) {
	mk := func(s string) net.Addr {
		ip, ipn, _ := net.ParseCIDR(s)
		ipn.IP = ip
		return ipn
	}
	got := pickAddrs([]net.Addr{mk("fe80::1/64"), mk("2001:db8::5/64"), mk("192.168.1.9/24"), mk("10.0.0.1/8")})
	if want := []string{"192.168.1.9", "2001:db8::5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSortNets(t *testing.T) {
	ns := []NetStat{{Name: "vEthernet", Default: true}, {Name: "Ethernet", Physical: true}, {Name: "Wi-Fi", Physical: true, Default: true}}
	sortNets(ns)
	if ns[0].Name != "Wi-Fi" || ns[1].Name != "vEthernet" || ns[2].Name != "Ethernet" {
		t.Errorf("order: %v %v %v", ns[0].Name, ns[1].Name, ns[2].Name)
	}
}

func TestSampleNets(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	s.Sample(ctx)
	time.Sleep(300 * time.Millisecond)
	snap, _, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Nets) == 0 {
		t.Fatal("no interfaces")
	}
	for _, n := range snap.Nets {
		t.Logf("%-34q default=%-5v phys=%-5v up=%-5v speed=%d rx=%.0f tx=%.0f %v %q",
			n.Name, n.Default, n.Physical, n.Up, n.SpeedBps, n.RecvBps, n.SentBps, n.Addrs, n.Desc)
	}
}
