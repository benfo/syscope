package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

// netRateFloor keeps idle adapters from showing noise as full-height graphs.
const netRateFloor = 12_500 // 100 Kbps

type netPage struct {
	m model
	n metrics.NetStat
}

func netPages(m model) []devicePage {
	out := make([]devicePage, len(m.snap.Nets))
	for i, n := range m.snap.Nets {
		out[i] = netPage{m, n}
	}
	return out
}

func (p netPage) key() string           { return "net:" + p.n.Name }
func (p netPage) name() string          { return p.n.Name }
func (p netPage) helpTitle() string     { return p.n.Name }
func (p netPage) hidden() bool          { return !p.n.Default }
func (p netPage) style() lipgloss.Style { return ui.network }

func (p netPage) record() {
	if p.n.RatesKnown {
		p.m.histories.push(p.key(), "rx", p.n.RecvBps)
		p.m.histories.push(p.key(), "tx", p.n.SentBps)
	}
}

func (p netPage) series(name string) []float64 { return p.m.histories.values(p.key(), name) }

func (p netPage) trend() []float64 {
	t, _ := scaled(netRateFloor, sumSeries(p.series("rx"), p.series("tx")))
	return t
}

func (p netPage) summary() string {
	return "↓" + view.FormatBitRate(p.n.RecvBps) + " ↑" + view.FormatBitRate(p.n.SentBps)
}

func (p netPage) detail(w, h int) []string {
	n := p.n
	rx, tx := p.series("rx"), p.series("tx")
	// Receive and send share one scale so they're comparable at a glance.
	_, top := scaled(netRateFloor, rx, tx)
	rxs, _ := scaled(top, rx)
	txs, _ := scaled(top, tx)
	graphs := []graphSpec{
		{title: "Receive", vals: rxs, style: ui.receive},
		{title: "Send", vals: txs, style: ui.send},
	}

	kind := "Virtual"
	if n.Physical {
		kind = "Hardware"
	}
	link := "--"
	if n.SpeedBps > 0 {
		link = view.FormatBitRate(float64(n.SpeedBps) / 8)
	}
	state := "Up"
	if !n.Up {
		state = "Down"
	}
	tail := append([]string{""}, packLine([]string{
		kv("Receive", view.FormatBitRate(n.RecvBps)), kv("Send", view.FormatBitRate(n.SentBps)),
		kv("Link", link), kv("Type", kind), kv("State", state),
	}, statSep, w)...)
	if len(n.Addrs) > 0 {
		tail = append(tail, kv("Address", strings.Join(n.Addrs, "  ")))
	}
	head := []string{
		spread(ui.label.Render(n.Name), ui.muted.Render(n.Desc), w),
		ui.muted.Render("Throughput · scale " + view.FormatBitRate(top) + " · " + p.m.graphSpan(w-2)),
	}
	return withGraphs(head, tail, graphs, w, h)
}

func (p netPage) terms() []term {
	return []term{
		{"Receive / Send", "Data in and out, in bits per second, the same unit as internet plans (100 Mbps ≈ 12.5 MB/s)."},
		{"Link", "Speed negotiated with your router or switch. It's the ceiling for this adapter, not your internet speed. On Wi-Fi it changes with signal strength."},
		{"Type", "Hardware is a physical Wi-Fi or Ethernet adapter. Virtual is VPN, Hyper-V, WSL or Docker; that traffic usually also passes through a hardware adapter, so it's counted twice."},
		{"Address", "This machine's IP address on that network. 169.254.x.x means it didn't get an address (not properly connected)."},
		{"Shown adapters", "By default only adapters that are up and either physical or carrying traffic. Press . to show all."},
		{"Graph", "Receive and Send share one scale so you can compare them; the scale is shown in the subtitle."},
	}
}
