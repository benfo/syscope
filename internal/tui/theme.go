package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme maps every colour role in the UI to a terminal colour. Presets and
// user themes (see "Theming" in ROADMAP.md) only need to provide one.
// Colours are ANSI indexes ("0"–"255") or hex ("#ff8800").
type Theme struct {
	Name string

	Accent lipgloss.Color // labels, selection marker
	Muted  lipgloss.Color // secondary text, borders, empty meter cells

	Good lipgloss.Color // load levels: low,
	Warn lipgloss.Color // high,
	Bad  lipgloss.Color // critical; also errors

	SelectionBg lipgloss.Color
	SelectionFg lipgloss.Color

	// Device graphs.
	CPU     lipgloss.Color
	Memory  lipgloss.Color
	Cached  lipgloss.Color
	Disk    lipgloss.Color
	Network lipgloss.Color
	GPU     lipgloss.Color

	// Paired series that appear together and must be told apart.
	Read    lipgloss.Color
	Write   lipgloss.Color
	Receive lipgloss.Color
	Send    lipgloss.Color

	PCore lipgloss.Color // hybrid CPU performance cores
	ECore lipgloss.Color // and efficiency cores
}

// DefaultTheme uses the 16 basic ANSI colours so it follows the user's
// terminal palette.
var DefaultTheme = Theme{
	Name:   "default",
	Accent: "6", Muted: "8",
	Good: "2", Warn: "3", Bad: "1",
	SelectionBg: "4", SelectionFg: "15",
	CPU: "6", Memory: "5", Cached: "4", Disk: "2", Network: "3", GPU: "4",
	Read: "2", Write: "3", Receive: "6", Send: "5",
	PCore: "5", ECore: "6",
}

// styles are the lipgloss styles derived from a Theme; rendering code uses
// these roles, never raw colours.
type styles struct {
	label    lipgloss.Style // section and metric labels
	accent   lipgloss.Style
	muted    lipgloss.Style
	bold     lipgloss.Style
	key      lipgloss.Style // key names in hints and help
	inverse  lipgloss.Style // table heading, active tab
	selected lipgloss.Style // selected table row
	prompt   lipgloss.Style // confirmations, active filter
	frame    lipgloss.Style // graph borders

	good, warn, bad lipgloss.Style

	cpu, memory, cached, disk, network, gpu lipgloss.Style
	read, write, receive, send              lipgloss.Style
	pCore, eCore                            lipgloss.Style
}

func newStyles(t Theme) styles {
	fg := func(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	return styles{
		label:    fg(t.Accent).Bold(true),
		accent:   fg(t.Accent),
		muted:    fg(t.Muted),
		bold:     lipgloss.NewStyle().Bold(true),
		key:      lipgloss.NewStyle().Bold(true),
		inverse:  lipgloss.NewStyle().Bold(true).Reverse(true),
		selected: lipgloss.NewStyle().Background(t.SelectionBg).Foreground(t.SelectionFg),
		prompt:   fg(t.Warn).Bold(true),
		frame:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Muted),

		good: fg(t.Good), warn: fg(t.Warn), bad: fg(t.Bad),

		cpu: fg(t.CPU), memory: fg(t.Memory), cached: fg(t.Cached),
		disk: fg(t.Disk), network: fg(t.Network), gpu: fg(t.GPU),
		read: fg(t.Read), write: fg(t.Write), receive: fg(t.Receive), send: fg(t.Send),
		pCore: fg(t.PCore).Bold(true), eCore: fg(t.ECore),
	}
}

// ui holds the active styles; see applyTheme.
var ui = newStyles(DefaultTheme)

// Themes are the built-in presets, in the order T cycles through them.
// Colours follow each palette's published values.
var Themes = []Theme{
	DefaultTheme,
	{
		Name: "dracula", Accent: "#bd93f9", Muted: "#6272a4",
		Good: "#50fa7b", Warn: "#f1fa8c", Bad: "#ff5555",
		SelectionBg: "#44475a", SelectionFg: "#f8f8f2",
		CPU: "#8be9fd", Memory: "#bd93f9", Cached: "#6272a4", Disk: "#50fa7b", Network: "#ffb86c", GPU: "#ff79c6",
		Read: "#50fa7b", Write: "#ffb86c", Receive: "#8be9fd", Send: "#ff79c6",
		PCore: "#ff79c6", ECore: "#8be9fd",
	},
	{
		Name: "catppuccin-mocha", Accent: "#cba6f7", Muted: "#6c7086",
		Good: "#a6e3a1", Warn: "#f9e2af", Bad: "#f38ba8",
		SelectionBg: "#45475a", SelectionFg: "#cdd6f4",
		CPU: "#89b4fa", Memory: "#cba6f7", Cached: "#b4befe", Disk: "#a6e3a1", Network: "#fab387", GPU: "#f5c2e7",
		Read: "#a6e3a1", Write: "#fab387", Receive: "#89dceb", Send: "#f5c2e7",
		PCore: "#cba6f7", ECore: "#94e2d5",
	},
	{
		Name: "tokyo-night", Accent: "#7aa2f7", Muted: "#565f89",
		Good: "#9ece6a", Warn: "#e0af68", Bad: "#f7768e",
		SelectionBg: "#33467c", SelectionFg: "#c0caf5",
		CPU: "#7dcfff", Memory: "#bb9af7", Cached: "#7aa2f7", Disk: "#9ece6a", Network: "#ff9e64", GPU: "#73daca",
		Read: "#9ece6a", Write: "#ff9e64", Receive: "#7dcfff", Send: "#bb9af7",
		PCore: "#bb9af7", ECore: "#7dcfff",
	},
	{
		Name: "nord", Accent: "#88c0d0", Muted: "#616e88",
		Good: "#a3be8c", Warn: "#ebcb8b", Bad: "#bf616a",
		SelectionBg: "#434c5e", SelectionFg: "#eceff4",
		CPU: "#88c0d0", Memory: "#b48ead", Cached: "#5e81ac", Disk: "#a3be8c", Network: "#d08770", GPU: "#81a1c1",
		Read: "#a3be8c", Write: "#d08770", Receive: "#88c0d0", Send: "#b48ead",
		PCore: "#b48ead", ECore: "#88c0d0",
	},
	{
		Name: "gruvbox-dark", Accent: "#fe8019", Muted: "#928374",
		Good: "#b8bb26", Warn: "#fabd2f", Bad: "#fb4934",
		SelectionBg: "#504945", SelectionFg: "#ebdbb2",
		CPU: "#83a598", Memory: "#d3869b", Cached: "#458588", Disk: "#b8bb26", Network: "#fe8019", GPU: "#8ec07c",
		Read: "#b8bb26", Write: "#fe8019", Receive: "#83a598", Send: "#d3869b",
		PCore: "#d3869b", ECore: "#8ec07c",
	},
	{
		Name: "solarized-dark", Accent: "#268bd2", Muted: "#586e75",
		Good: "#859900", Warn: "#b58900", Bad: "#dc322f",
		SelectionBg: "#073642", SelectionFg: "#93a1a1",
		CPU: "#2aa198", Memory: "#d33682", Cached: "#6c71c4", Disk: "#859900", Network: "#cb4b16", GPU: "#268bd2",
		Read: "#859900", Write: "#cb4b16", Receive: "#2aa198", Send: "#d33682",
		PCore: "#d33682", ECore: "#2aa198",
	},
	{
		// For light terminal backgrounds (GitHub Light palette).
		Name: "github-light", Accent: "#0969da", Muted: "#6e7781",
		Good: "#1a7f37", Warn: "#9a6700", Bad: "#cf222e",
		SelectionBg: "#ddf4ff", SelectionFg: "#1f2328",
		CPU: "#0969da", Memory: "#8250df", Cached: "#54aeff", Disk: "#1a7f37", Network: "#bc4c00", GPU: "#bf3989",
		Read: "#1a7f37", Write: "#bc4c00", Receive: "#0969da", Send: "#8250df",
		PCore: "#8250df", ECore: "#0969da",
	},
	{
		// Bright ANSI colours, white selection: maximum legibility.
		Name: "high-contrast", Accent: "14", Muted: "7",
		Good: "10", Warn: "11", Bad: "9",
		SelectionBg: "15", SelectionFg: "0",
		CPU: "14", Memory: "13", Cached: "12", Disk: "10", Network: "11", GPU: "12",
		Read: "10", Write: "11", Receive: "14", Send: "13",
		PCore: "13", ECore: "14",
	},
	{
		// Okabe–Ito palette: distinguishable with the common colour-vision
		// deficiencies (good/warn/bad avoid relying on red vs green).
		Name: "colorblind", Accent: "#56b4e9", Muted: "#999999",
		Good: "#0072b2", Warn: "#e69f00", Bad: "#d55e00",
		SelectionBg: "#0072b2", SelectionFg: "#ffffff",
		CPU: "#56b4e9", Memory: "#cc79a7", Cached: "#0072b2", Disk: "#009e73", Network: "#e69f00", GPU: "#f0e442",
		Read: "#009e73", Write: "#e69f00", Receive: "#56b4e9", Send: "#cc79a7",
		PCore: "#cc79a7", ECore: "#56b4e9",
	},
}

// ThemeByName finds a built-in theme.
func ThemeByName(name string) (Theme, bool) {
	for _, t := range Themes {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Theme{}, false
}

// ThemeNames lists the built-in themes, for help text and errors.
func ThemeNames() []string {
	names := make([]string, len(Themes))
	for i, t := range Themes {
		names[i] = t.Name
	}
	return names
}

// applyTheme makes t the active theme. Styles are package state because
// every render function uses them; this is the only place that changes them.
func applyTheme(t Theme) { ui = newStyles(t) }

// Swatch renders a one-line preview of a theme: its name and a coloured
// block per main role.
func Swatch(t Theme) string {
	roles := []struct {
		name string
		c    lipgloss.Color
	}{
		{"accent", t.Accent}, {"good", t.Good}, {"warn", t.Warn}, {"bad", t.Bad},
		{"cpu", t.CPU}, {"mem", t.Memory}, {"disk", t.Disk}, {"net", t.Network}, {"gpu", t.GPU},
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%-18s", t.Name))
	for _, r := range roles {
		b.WriteString(lipgloss.NewStyle().Foreground(r.c).Render("██ "+r.name) + "  ")
	}
	return b.String()
}
