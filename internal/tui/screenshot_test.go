package tui

import (
	"context"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"github.com/benfo/syscope/internal/metrics"
)

// README screenshots are rendered in colour from synthetic data, like the
// golden screens, never from a live machine (the repo is public). Regenerate
// them with:  go test ./internal/tui -run Screenshots -screenshots
var screenshots = flag.Bool("screenshots", false, "write the README screenshots to docs/screenshots")

const screenshotDir = "../../docs/screenshots"

func TestScreenshots(t *testing.T) {
	if !*screenshots {
		t.Skip("run with -screenshots to render the README screenshots")
	}
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	shots := []struct {
		name string
		keys []tea.KeyMsg
	}{
		{"processes", []tea.KeyMsg{key("t")}},
		{"performance", []tea.KeyMsg{key("2")}},
		{"gpu", []tea.KeyMsg{key("2"), key("G"), key("k")}},
		{"ports", []tea.KeyMsg{key("3"), key(".")}},
	}
	if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range shots {
		m := screenshotModel(t)
		for _, k := range s.keys {
			m = drive(t, m, k)
		}
		svg := screenSVG(m.View())
		if err := os.WriteFile(filepath.Join(screenshotDir, s.name+".svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// screenshotSampler is a busier machine than fakeSampler: 16 cores and a
// realistic process list, with load that varies from sample to sample.
type screenshotSampler struct {
	fakeSampler
	n int
}

func (s *screenshotSampler) Sample(ctx context.Context) (metrics.SystemSnapshot, []metrics.ProcInfo, error) {
	snap, procs, err := s.fakeSampler.Sample(ctx)
	s.n++
	i := s.n
	snap.NumCPU = 16
	snap.CPUPerCore = make([]float64, 16)
	for c := range snap.CPUPerCore {
		snap.CPUPerCore[c] = float64((c*23 + i*17) % 90)
	}
	snap.CPUTotal = float64(20 + (i*37)%45)
	snap.MemUsed = uint64(38+i%6) << 30
	snap.Disks[0].ActivePct = float64((i * 13) % 60)
	snap.Disks[0].ReadBps = float64(i*7%10) * 1e6
	snap.Nets[0].RecvBps = float64(i*11%17) * 4e5
	snap.Nets[0].SentBps = float64(i*5%7) * 6e4
	snap.GPUs[0].Util = float64(40 + (i*29)%55)
	snap.GPUs[0].Engines[0].Util = snap.GPUs[0].Util
	snap.Sockets = screenshotSockets()
	return snap, procs, err
}

// screenshotSockets belong to screenshotProcs, with documentation addresses.
func screenshotSockets() []metrics.Socket {
	return []metrics.Socket{
		{Proto: "TCP", LocalIP: "127.0.0.1", LocalPort: 11434, State: "LISTEN", PID: 4120},
		{Proto: "TCP", LocalIP: "0.0.0.0", LocalPort: 445, State: "LISTEN", PID: 4},
		{Proto: "TCP", LocalIP: "127.0.0.1", LocalPort: 8080, State: "LISTEN", PID: 8061},
		{Proto: "UDP", LocalIP: "0.0.0.0", LocalPort: 5353, PID: 6012},
		{Proto: "UDP", LocalIP: "0.0.0.0", LocalPort: 3478, PID: 9512},
		{Proto: "UDP", LocalIP: "0.0.0.0", LocalPort: 27015, PID: 9300},
		{Proto: "TCP", LocalIP: "0.0.0.0", LocalPort: 135, State: "LISTEN", PID: 1460},
		{Proto: "TCP", LocalIP: "192.0.2.106", LocalPort: 52144, RemoteIP: "203.0.113.20", RemotePort: 443, State: "ESTABLISHED", PID: 6012},
		{Proto: "TCP", LocalIP: "192.0.2.106", LocalPort: 52160, RemoteIP: "203.0.113.45", RemotePort: 443, State: "ESTABLISHED", PID: 6140},
		{Proto: "TCP", LocalIP: "192.0.2.106", LocalPort: 52171, RemoteIP: "198.51.100.30", RemotePort: 27015, State: "ESTABLISHED", PID: 9300},
		{Proto: "TCP", LocalIP: "127.0.0.1", LocalPort: 52150, RemoteIP: "127.0.0.1", RemotePort: 11434, State: "ESTABLISHED", PID: 7710},
		{Proto: "TCP", LocalIP: "192.0.2.106", LocalPort: 52101, RemoteIP: "198.51.100.9", RemotePort: 443, State: "TIME_WAIT"},
	}
}

func screenshotModel(t *testing.T) model {
	s := &screenshotSampler{fakeSampler: fakeSampler{procs: screenshotProcs()}}
	kinds := make([]metrics.CoreKind, 16)
	for c := range kinds {
		kinds[c] = metrics.CorePerformance
		if c >= 8 {
			kinds[c] = metrics.CoreEfficiency
		}
	}
	m := newModel(Options{Sampler: s, Details: fakeDetails, Now: fakeNow, CPUInfo: metrics.CPUInfo{
		Model: "Example 16-Core Processor", Logical: 16, Physical: 16, BaseMHz: 2400, Kinds: kinds}})
	m = drive(t, m, tea.WindowSizeMsg{Width: 110, Height: 32})
	for range 200 {
		snap, procs, _ := s.Sample(context.Background())
		m = drive(t, m, sampleMsg{snap: snap, procs: procs})
	}
	return m
}

// screenshotProcs is a made-up desktop: a local LLM server, a browser with
// helpers, a game, developer tools and system processes.
func screenshotProcs() []metrics.ProcInfo {
	const user, sys = `CORP\sam`, "SYSTEM"
	mib := uint64(1 << 20)
	proc := func(pid, ppid int32, name, owner string, cpu float64, rss uint64, gpu float64, vram uint64) metrics.ProcInfo {
		return metrics.ProcInfo{PID: pid, PPID: ppid, Name: name, User: owner, CreateTime: fakeNow().Add(-time.Duration(pid%300) * time.Minute),
			CPU: cpu, CPUKnown: true, RSS: rss * mib, RSSKnown: true, GPU: gpu, GPUKnown: true, VRAM: vram * mib, VRAMKnown: true}
	}
	return []metrics.ProcInfo{
		{PID: 4, Name: "System", User: sys},
		proc(812, 4, "services.exe", sys, 0.1, 9, 0, 0),
		proc(1460, 812, "svchost.exe", sys, 0.3, 38, 0, 0),
		proc(1932, 812, "svchost.exe", sys, 0.0, 21, 0, 0),
		proc(2210, 812, "svchost.exe", sys, 0.1, 64, 0, 0),
		proc(3108, 0, "dwm.exe", sys, 1.4, 142, 6.2, 380),
		proc(5520, 0, "explorer.exe", user, 0.4, 210, 0.3, 64),
		proc(4120, 5520, "ollama.exe", user, 6.8, 1180, 0, 0),
		proc(4388, 4120, "ollama_llama_server.exe", user, 18.5, 2650, 91.0, 9400),
		proc(6012, 5520, "browser.exe", user, 2.1, 640, 1.2, 220),
		proc(6140, 6012, "browser.exe", user, 0.9, 310, 0.4, 96),
		proc(6188, 6012, "browser.exe", user, 0.6, 180, 0, 0),
		proc(6232, 6012, "browser.exe", user, 0.2, 145, 0, 0),
		proc(7710, 5520, "Example.TextEditor.exe", user, 1.1, 720, 0.2, 48),
		proc(7744, 7710, "gopls.exe", user, 3.4, 890, 0, 0),
		proc(8020, 7710, "terminal.exe", user, 0.3, 96, 0.1, 24),
		proc(8061, 8020, "go.exe", user, 9.7, 410, 0, 0),
		proc(9300, 5520, "StarRacer.exe", user, 12.2, 3100, 0, 0),
		proc(9512, 5520, "Meet.exe", user, 1.6, 520, 2.8, 310),
		{PID: 9840, PPID: 5520, Name: "SongPlayer.exe", User: user, Status: "stop", CPUKnown: true, RSS: 187 * mib, RSSKnown: true, GPUKnown: true, VRAMKnown: true},
	}
}

// screenSVG draws a terminal screen as an SVG window on a fixed cell grid.
// Braille, block and box-drawing characters are drawn as shapes, so graphs
// and frames look the same whatever fonts the viewer has; text is stretched
// to its cells so a fallback font can't shift the layout.
func screenSVG(screen string) string {
	const (
		cw, ch     = 8.4, 18.0 // cell size
		pad, title = 20.0, 30.0
		bg, fg     = "#171717", "#c4c4c4"
		font       = `'JetBrains Mono','Cascadia Mono','SF Mono',Menlo,Consolas,'DejaVu Sans Mono',monospace`
	)
	grid := parseScreen(screen)
	cols := 0
	for _, row := range grid {
		cols = max(cols, len(row))
	}
	w, h := float64(cols)*cw+2*pad, float64(len(grid))*ch+pad+title

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g">`+"\n", w, h, w, h)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" rx="8" fill="%s"/>`+"\n", bg)
	for i, c := range []string{"#FF5A54", "#E6BF29", "#52C12B"} {
		fmt.Fprintf(&b, `<circle cx="%g" cy="15" r="5.5" fill="%s"/>`+"\n", 18+float64(i)*19, c)
	}
	fmt.Fprintf(&b, `<g font-family="%s" font-size="14" fill="%s">`+"\n", font, fg)
	// Backgrounds first, one rect per run of cells, so there are no seams.
	for r, row := range grid {
		for c := 0; c < len(row); {
			end := c
			for end < len(row) && row[end].bg == row[c].bg {
				end++
			}
			if row[c].bg != "" {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`+"\n",
					pad+float64(c)*cw, title+float64(r)*ch, float64(end-c)*cw, ch, row[c].bg)
			}
			c = end
		}
	}
	for r, row := range grid {
		y := title + float64(r)*ch
		for c := 0; c < len(row); {
			cell := row[c]
			x := pad + float64(c)*cw
			color := cell.fg
			if color == "" {
				color = fg
			}
			if shape := cellShape(cell.r, x, y, cw, ch); shape != "" {
				fmt.Fprintf(&b, `<g fill="%s" color="%s"%s>%s</g>`+"\n", color, color, opacity(cell), shape)
				c++
				continue
			}
			// A run of text cells in the same style becomes one <text>.
			end, width := c, 0
			var text strings.Builder
			for end < len(row) && row[end].sameStyle(cell) && cellShape(row[end].r, 0, 0, cw, ch) == "" {
				if row[end].width > 0 {
					text.WriteRune(row[end].r)
				}
				width += row[end].width
				end++
			}
			if s := text.String(); strings.TrimSpace(s) != "" {
				weight := ""
				if cell.bold {
					weight = ` font-weight="bold"`
				}
				fmt.Fprintf(&b, `<text x="%g" y="%g" textLength="%g" lengthAdjust="spacingAndGlyphs" xml:space="preserve" fill="%s"%s%s>%s</text>`+"\n",
					x, y+ch*0.72, float64(width)*cw, color, weight, opacity(cell), html.EscapeString(s))
			}
			c = end
		}
	}
	b.WriteString("</g>\n</svg>\n")
	return b.String()
}

type screenCell struct {
	r           rune
	width       int // 0 for the second half of a wide character
	fg, bg      string
	bold, faint bool
}

func (c screenCell) sameStyle(o screenCell) bool {
	return c.fg == o.fg && c.bg == o.bg && c.bold == o.bold && c.faint == o.faint
}

func opacity(c screenCell) string {
	if c.faint {
		return ` opacity="0.6"`
	}
	return ""
}

// ansiColors is the screenshot palette for the 16 ANSI colours.
var ansiColors = [16]string{
	"#2e2e2e", "#D74E6F", "#31BB71", "#D3E561", "#8056FF", "#ED61D7", "#04D7D7", "#c4c4c4",
	"#7a7a7a", "#FE5F86", "#00D787", "#EBFF71", "#9B79FF", "#FF7AEA", "#00FEFE", "#FFFFFF",
}

// parseScreen splits a rendered screen into cells, applying SGR colour codes.
func parseScreen(screen string) [][]screenCell {
	var grid [][]screenCell
	for _, line := range strings.Split(strings.TrimRight(screen, "\n"), "\n") {
		var row []screenCell
		var cur screenCell
		var reverse bool
		for i := 0; i < len(line); {
			if strings.HasPrefix(line[i:], "\x1b[") {
				j := i + 2
				for j < len(line) && (line[j] < '@' || line[j] > '~') {
					j++
				}
				if j < len(line) && line[j] == 'm' {
					applySGR(&cur, &reverse, line[i+2:j])
				}
				i = j + 1
				continue
			}
			r, size := []rune(line[i:])[0], len(string([]rune(line[i:])[0]))
			i += size
			cell := cur
			if reverse {
				cell.fg, cell.bg = cell.bg, cell.fg
				if cell.fg == "" {
					cell.fg = "#171717"
				}
				if cell.bg == "" {
					cell.bg = "#c4c4c4"
				}
			}
			cell.r, cell.width = r, runewidth.RuneWidth(r)
			row = append(row, cell)
			if cell.width == 2 {
				pad := cell
				pad.width = 0
				row = append(row, pad)
			}
		}
		grid = append(grid, row)
	}
	return grid
}

func applySGR(c *screenCell, reverse *bool, params string) {
	codes := strings.Split(params, ";")
	for k := 0; k < len(codes); k++ {
		n, _ := strconv.Atoi(codes[k])
		switch {
		case n == 0:
			*c, *reverse = screenCell{}, false
		case n == 1:
			c.bold = true
		case n == 2:
			c.faint = true
		case n == 22:
			c.bold, c.faint = false, false
		case n == 7:
			*reverse = true
		case n == 27:
			*reverse = false
		case n >= 30 && n <= 37:
			c.fg = ansiColors[n-30]
		case n >= 90 && n <= 97:
			c.fg = ansiColors[n-90+8]
		case n == 39:
			c.fg = ""
		case n >= 40 && n <= 47:
			c.bg = ansiColors[n-40]
		case n >= 100 && n <= 107:
			c.bg = ansiColors[n-100+8]
		case n == 49:
			c.bg = ""
		case (n == 38 || n == 48) && k+4 < len(codes) && codes[k+1] == "2":
			r, _ := strconv.Atoi(codes[k+2])
			g, _ := strconv.Atoi(codes[k+3])
			bl, _ := strconv.Atoi(codes[k+4])
			hex := fmt.Sprintf("#%02x%02x%02x", r, g, bl)
			if n == 38 {
				c.fg = hex
			} else {
				c.bg = hex
			}
			k += 4
		}
	}
}

// cellShape draws Braille, block and box-drawing characters as SVG shapes
// filling the cell at (x, y), or returns "" for ordinary text.
func cellShape(r rune, x, y, cw, ch float64) string {
	rect := func(rx, ry, rw, rh float64) string {
		return fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`, rx, ry, rw, rh)
	}
	switch {
	case r >= 0x2801 && r <= 0x28FF: // Braille: dots 1-3 and 7 on the left, 4-6 and 8 on the right
		var s strings.Builder
		dots := [8][2]int{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}, {0, 3}, {1, 3}}
		for bit, d := range dots {
			if (r-0x2800)&(1<<bit) != 0 {
				fmt.Fprintf(&s, `<circle cx="%.1f" cy="%.1f" r="%.1f"/>`,
					x+cw*(0.27+0.46*float64(d[0])), y+ch*(0.125+0.25*float64(d[1])), cw*0.17)
			}
		}
		return s.String()
	case r >= 0x2581 && r <= 0x2588: // lower eighths
		hh := ch * float64(r-0x2580) / 8
		return rect(x, y+ch-hh, cw, hh)
	case r == 0x2580: // upper half
		return rect(x, y, cw, ch/2)
	case r >= 0x2589 && r <= 0x258F: // left seven eighths down to one eighth
		return rect(x, y, cw*float64(0x2590-r)/8, ch)
	case r == 0x2590: // right half
		return rect(x+cw/2, y, cw/2, ch)
	case r >= 0x2591 && r <= 0x2593: // shades
		return fmt.Sprintf(`<g opacity="%.2f">%s</g>`, 0.25*float64(r-0x2590), rect(x, y, cw, ch))
	}
	return boxShape(r, x, y, cw, ch)
}

// boxShape draws box-drawing lines from the cell centre to the edges named
// by each character; rounded corners curve through the centre.
func boxShape(r rune, x, y, cw, ch float64) string {
	type dirs struct{ up, down, left, right, round bool }
	box := map[rune]dirs{
		'─': {left: true, right: true}, '│': {up: true, down: true},
		'┌': {down: true, right: true}, '┐': {down: true, left: true}, '└': {up: true, right: true}, '┘': {up: true, left: true},
		'╭': {down: true, right: true, round: true}, '╮': {down: true, left: true, round: true},
		'╰': {up: true, right: true, round: true}, '╯': {up: true, left: true, round: true},
		'├': {up: true, down: true, right: true}, '┤': {up: true, down: true, left: true},
		'┬': {left: true, right: true, down: true}, '┴': {left: true, right: true, up: true},
		'┼': {up: true, down: true, left: true, right: true},
	}
	d, ok := box[r]
	if !ok {
		return ""
	}
	cx, cy := x+cw/2, y+ch/2
	line := func(x1, y1, x2, y2 float64) string {
		return fmt.Sprintf(`<path d="M%.1f %.1fL%.1f %.1f" fill="none" stroke="currentColor" stroke-width="1.2"/>`, x1, y1, x2, y2)
	}
	if d.round {
		ex, ey := x, cy // horizontal edge point
		if d.right {
			ex = x + cw
		}
		vy := y // vertical edge point
		if d.down {
			vy = y + ch
		}
		return fmt.Sprintf(`<path d="M%.1f %.1fQ%.1f %.1f %.1f %.1f" fill="none" stroke="currentColor" stroke-width="1.2"/>`, cx, vy, cx, cy, ex, ey)
	}
	var s strings.Builder
	if d.up {
		s.WriteString(line(cx, y, cx, cy))
	}
	if d.down {
		s.WriteString(line(cx, cy, cx, y+ch))
	}
	if d.left {
		s.WriteString(line(x, cy, cx, cy))
	}
	if d.right {
		s.WriteString(line(cx, cy, x+cw, cy))
	}
	return s.String()
}
