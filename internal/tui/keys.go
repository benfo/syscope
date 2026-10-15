package tui

// action is something a key does. Every binding lives in keymap below, which
// also drives the footer hints, the ? help overlay and the keymap tests, so
// keys can't drift between features. See "Key rules" in ROADMAP.md.
type action int

const (
	actNone action = iota
	actDown
	actUp
	actTop
	actBottom
	actHalfDown
	actHalfUp
	actPageDown
	actPageUp
	actTab1
	actTab2
	actTab3
	actNextTab
	actPrevTab
	actHelp
	actQuit
	actCancel
	actSearch
	actEndTask
	actForceKill
	actSortNext
	actSortReverse
	actGroup
	actCPUMode
	actShowHidden
	actDetails
	actSettings
	actTree
	actExpand
	actCollapse
	actExpandAll
	actCollapseAll
	actGoToProcess
	actSuspend
	numActions // sentinel for tests
)

type scope int

const (
	scopeGlobal scope = iota
	scopeProcesses
	scopePerformance
	scopePorts
)

var scopeTitles = map[scope]string{
	scopeGlobal:      "Everywhere",
	scopeProcesses:   "Processes",
	scopePerformance: "Performance",
	scopePorts:       "Ports",
}

// binding is one row of help: a label, a description and the keys it maps.
type binding struct {
	scope scope
	label string
	desc  string
	short string // footer hint; empty = not shown in the footer
	keys  map[string]action
}

var keymap = []binding{
	{scopeGlobal, "j/k", "move down / up", "move", map[string]action{"j": actDown, "down": actDown, "k": actUp, "up": actUp}},
	{scopeGlobal, "g/G", "top / bottom", "", map[string]action{"g": actTop, "home": actTop, "G": actBottom, "end": actBottom}},
	{scopeGlobal, "^d/^u", "half page down / up", "", map[string]action{
		"ctrl+d": actHalfDown, "ctrl+u": actHalfUp, "pgdown": actPageDown, "pgup": actPageUp}},
	{scopeGlobal, "1-3", "jump to tab", "tabs", map[string]action{"1": actTab1, "2": actTab2, "3": actTab3}},
	{scopeGlobal, "tab", "next / previous tab", "", map[string]action{"tab": actNextTab, "shift+tab": actPrevTab}},
	{scopeGlobal, "esc", "close pane / cancel / clear search", "", map[string]action{"esc": actCancel}},
	{scopeGlobal, ",", "settings: theme, refresh, defaults, restore session", "settings", map[string]action{",": actSettings}},
	{scopeGlobal, "?", "help", "help", map[string]action{"?": actHelp}},
	{scopeGlobal, "q", "quit", "quit", map[string]action{"q": actQuit, "ctrl+c": actQuit}},

	{scopeProcesses, "/", "search (live filter)", "search", map[string]action{"/": actSearch}},
	{scopeProcesses, "d/D", "end task / force kill (t: also the processes it started)", "kill", map[string]action{"d": actEndTask, "delete": actEndTask, "D": actForceKill}},
	{scopeProcesses, "s/S", "next sort column / reverse", "sort", map[string]action{"s": actSortNext, "S": actSortReverse}},
	{scopeProcesses, "z", "group by app (again: flat)", "group", map[string]action{"z": actGroup}},
	{scopeProcesses, "t", "process tree (again: flat)", "tree", map[string]action{"t": actTree}},
	{scopeProcesses, "l/h", "expand / collapse (or go to parent)", "fold", map[string]action{
		"l": actExpand, "right": actExpand, "h": actCollapse, "left": actCollapse}},
	{scopeProcesses, "L/H", "expand / collapse all", "", map[string]action{"L": actExpandAll, "H": actCollapseAll}},
	{scopeProcesses, "c", "CPU% normalized / per-core", "cpu%", map[string]action{"c": actCPUMode}},
	{scopeProcesses, "p", "suspend (asks) / resume", "suspend", map[string]action{"p": actSuspend}},
	{scopeProcesses, "enter", "details pane (follows the cursor)", "details", map[string]action{"enter": actDetails}},

	{scopePerformance, ".", "show hidden adapters", "hidden", map[string]action{".": actShowHidden}},

	{scopePorts, "/", "search by port, name, PID or address", "search", map[string]action{"/": actSearch}},
	{scopePorts, "d/D", "end / force kill the owner (t: also processes it started)", "kill", map[string]action{
		"d": actEndTask, "delete": actEndTask, "D": actForceKill}},
	{scopePorts, "s/S", "next sort column / reverse", "sort", map[string]action{"s": actSortNext, "S": actSortReverse}},
	{scopePorts, ".", "show connections too, not just listening", "all", map[string]action{".": actShowHidden}},
	{scopePorts, "enter", "go to the owning process", "process", map[string]action{"enter": actGoToProcess}},
}

// lookup resolves a key in the given scope, falling back to global keys.
func lookup(sc scope, key string) action {
	for _, want := range []scope{sc, scopeGlobal} {
		for _, b := range keymap {
			if b.scope == want {
				if a, ok := b.keys[key]; ok {
					return a
				}
			}
		}
	}
	return actNone
}

func (m model) scope() scope {
	switch m.tab {
	case tabPerformance:
		return scopePerformance
	case tabPorts:
		return scopePorts
	}
	return scopeProcesses
}

// hints is the footer line for a scope: its own hinted keys, then global ones.
func hints(sc scope) []binding {
	var out []binding
	for _, want := range []scope{sc, scopeGlobal} {
		for _, b := range keymap {
			if b.scope == want && b.short != "" {
				out = append(out, b)
			}
		}
	}
	return out
}

// helpLines renders the ? overlay: the current tab's keys first, then global.
func helpLines(sc scope) []string {
	var out []string
	for _, want := range []scope{sc, scopeGlobal} {
		out = append(out, ui.label.Render("Keys: "+scopeTitles[want]))
		for _, b := range keymap {
			if b.scope == want {
				out = append(out, "  "+ui.key.Render(fit(b.label, 8))+" "+b.desc)
			}
		}
		out = append(out, "")
	}
	out = append(out, ui.muted.Render("Arrows, PgUp/PgDn and Home/End always work too."))
	return out
}

// menuMove applies a movement action to a menu: a short list shown in full
// (Settings, the theme list, the Performance devices). Menus wrap around at
// either end; long scrolling lists (processes, help) don't, and use g/G
// instead. moved is false for actions that aren't movement.
func menuMove(sel, n int, act action) (next int, moved bool) {
	switch act {
	case actDown, actHalfDown, actPageDown:
		return wrap(sel+1, n), true
	case actUp, actHalfUp, actPageUp:
		return wrap(sel-1, n), true
	case actTop:
		return 0, true
	case actBottom:
		return n - 1, true
	}
	return sel, false
}

// wrap keeps i in [0, n), cycling past either end.
func wrap(i, n int) int { return ((i % n) + n) % n }

func isTabAction(a action) bool {
	return a == actTab1 || a == actTab2 || a == actTab3 || a == actNextTab || a == actPrevTab
}
