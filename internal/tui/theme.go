package tui

import "github.com/charmbracelet/lipgloss"

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

// ui holds the active styles. It is set once at startup; theme selection
// will set it from the chosen Theme.
var ui = newStyles(DefaultTheme)
