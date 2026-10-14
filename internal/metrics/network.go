package metrics

import (
	"context"
	"net"
	"sort"
	"time"

	gnet "github.com/shirou/gopsutil/v4/net"
)

// NetStat is one network interface.
type NetStat struct {
	Name     string
	Desc     string
	Physical bool // a hardware adapter (Wi-Fi, Ethernet) rather than VPN / Hyper-V / Docker
	Up       bool
	SpeedBps uint64 // link speed in bits/s; 0 when unknown
	Addrs    []string

	RatesKnown bool
	RecvBps    float64 // bytes/s
	SentBps    float64

	// Default reports whether the interface is shown without "show all":
	// up, not loopback, and either physical or has carried traffic since
	// the sampler started.
	Default bool
}

// netMeta is per-OS adapter information gopsutil doesn't provide.
type netMeta struct {
	Desc     string
	Physical bool
	SpeedBps uint64
}

func defaultVisible(up, loopback, physical, seenTraffic bool) bool {
	return up && !loopback && (physical || seenTraffic)
}

// networkCollector computes per-interface rates and remembers which
// interfaces have carried traffic, for the default visibility rule.
type networkCollector struct {
	prev   map[string]gnet.IOCountersStat
	prevAt time.Time
	seen   map[string]bool
}

func newNetworkCollector() *networkCollector {
	return &networkCollector{seen: map[string]bool{}}
}

func (c *networkCollector) collect(ctx context.Context, now time.Time, snap *SystemSnapshot) error {
	ctrs, err := gnet.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil // networks are optional
	}
	ifaces := map[string]net.Interface{}
	if list, err := net.Interfaces(); err == nil {
		for _, ifc := range list {
			ifaces[ifc.Name] = ifc
		}
	}
	meta := netMetadata()

	var wall time.Duration
	if !c.prevAt.IsZero() {
		wall = now.Sub(c.prevAt)
	}
	next := make(map[string]gnet.IOCountersStat, len(ctrs))
	out := make([]NetStat, 0, len(ctrs))
	for _, ctr := range ctrs {
		st := NetStat{Name: ctr.Name}
		loopback := false
		if ifc, ok := ifaces[ctr.Name]; ok {
			st.Up = ifc.Flags&net.FlagUp != 0 && ifc.Flags&net.FlagRunning != 0
			loopback = ifc.Flags&net.FlagLoopback != 0
			if addrs, err := ifc.Addrs(); err == nil {
				st.Addrs = pickAddrs(addrs)
			}
		}
		m := meta[ctr.Name]
		st.Desc, st.Physical, st.SpeedBps = m.Desc, m.Physical, m.SpeedBps

		if prev, ok := c.prev[ctr.Name]; ok && wall > 0 {
			st.RatesKnown = true
			st.RecvBps = float64(sub(ctr.BytesRecv, prev.BytesRecv)) / wall.Seconds()
			st.SentBps = float64(sub(ctr.BytesSent, prev.BytesSent)) / wall.Seconds()
			if st.RecvBps+st.SentBps > 0 {
				c.seen[ctr.Name] = true
			}
		}
		st.Default = defaultVisible(st.Up, loopback, st.Physical, c.seen[ctr.Name])
		next[ctr.Name] = ctr
		out = append(out, st)
	}
	c.prev, c.prevAt = next, now
	sortNets(out)
	snap.Nets = out
	return nil
}

// pickAddrs returns the first IPv4 and first global IPv6 address.
func pickAddrs(addrs []net.Addr) []string {
	var v4, v6 string
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipn.IP
		switch {
		case ip.To4() != nil && v4 == "":
			v4 = ip.String()
		case ip.To4() == nil && v6 == "" && ip.IsGlobalUnicast():
			v6 = ip.String()
		}
	}
	var out []string
	for _, a := range []string{v4, v6} {
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

// sortNets orders default-visible interfaces first, physical before
// virtual, then by name.
func sortNets(ns []NetStat) {
	sort.SliceStable(ns, func(i, j int) bool {
		a, b := ns[i], ns[j]
		if a.Default != b.Default {
			return a.Default
		}
		if a.Physical != b.Physical {
			return a.Physical
		}
		return a.Name < b.Name
	})
}
