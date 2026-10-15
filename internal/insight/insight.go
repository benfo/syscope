// Package insight explains why a process uses the resources it does, in
// plain language, from data syscope already collects. Each explanation is a
// rule in rules; adding a hint means adding a rule.
package insight

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

// Level says whether a hint explains (Info) or suggests acting (Warn).
type Level int

const (
	Info Level = iota
	Warn
)

type Hint struct {
	Level Level
	Text  string
}

// Context is everything a rule may look at for one process.
type Context struct {
	Proc     metrics.ProcInfo
	Details  metrics.ProcDetails // zero when not read yet
	SameName int                 // processes sharing this executable name, itself included
	NumCPU   int
	MemTotal uint64
}

type rule struct {
	level Level
	when  func(Context) bool
	text  func(Context) string
}

// For returns the hints that apply to a process, most important first.
func For(c Context) []Hint {
	var out []Hint
	for _, lvl := range []Level{Warn, Info} {
		for _, r := range rules {
			if r.level == lvl && r.when(c) {
				out = append(out, Hint{r.level, r.text(c)})
			}
		}
	}
	return out
}

const (
	mib             = 1 << 20
	cachedVRAMMin   = 256 * mib
	idleGPUPct      = 1.0
	llmBusyCores    = 1.0 // at least one full core
	llmIdleGPUPct   = 5.0
	sameNameMany    = 5
	bigMemoryPctMin = 25.0
)

var rules = []rule{
	{ // Paused by someone: by syscope (p), Ctrl+Z in a terminal, a debugger, or Windows itself.
		level: Info,
		when:  func(c Context) bool { return c.Proc.Suspended() },
		text: func(Context) string {
			return "Suspended: none of its threads run, so it uses no CPU but keeps its memory. Press p to resume it. " +
				"Windows suspends background Store apps by itself to save power; on Linux and macOS, Ctrl+Z in a terminal suspends a program the same way."
		},
	},
	{ // A zombie: exited, but its parent hasn't collected its exit status.
		level: Info,
		when:  func(c Context) bool { return c.Proc.Zombie() },
		text: func(Context) string {
			return "Already exited. It stays listed (a \"zombie\") until its parent, shown above, collects its exit status. " +
				"It uses no CPU or memory and can't be ended. If zombies pile up, the parent has a bug; ending the parent clears them."
		},
	},
	{ // A local LLM working the CPU while the GPU idles has usually fallen back.
		level: Warn,
		when: func(c Context) bool {
			p := c.Proc
			return isLLMRuntime(p.Name) && p.CPUKnown && cores(c) >= llmBusyCores &&
				p.GPUKnown && p.GPU < llmIdleGPUPct
		},
		text: func(c Context) string {
			return fmt.Sprintf("Busy on the CPU (%.1f cores) but barely using the GPU: the model has probably fallen back to the CPU, "+
				"usually because it didn't fit in VRAM. Check free VRAM on the GPU page.", cores(c))
		},
	},
	{
		level: Warn,
		when: func(c Context) bool {
			return c.Proc.RSSKnown && c.MemTotal > 0 && view.Percent(c.Proc.RSS, c.MemTotal) >= bigMemoryPctMin
		},
		text: func(c Context) string {
			return fmt.Sprintf("Using %.0f%% of your RAM.", view.Percent(c.Proc.RSS, c.MemTotal))
		},
	},
	{
		level: Info,
		when: func(c Context) bool {
			p := c.Proc
			return p.VRAMKnown && p.VRAM >= cachedVRAMMin && p.GPUKnown && p.GPU < idleGPUPct
		},
		text: func(c Context) string {
			return fmt.Sprintf("Holds %s of GPU memory while idle: cached graphics (textures, buffers), not active use. "+
				"It's reclaimed if another app needs the VRAM.", view.FormatBytes(c.Proc.VRAM))
		},
	},
	{
		level: Info,
		when:  func(c Context) bool { return strings.Contains(c.Proc.GPUEngine, "Video Encode") },
		text: func(Context) string {
			return "Encoding video on the GPU: normal for video calls, screen recording and streaming."
		},
	},
	{
		level: Info,
		when:  func(c Context) bool { return strings.Contains(c.Proc.GPUEngine, "Video Decode") },
		text:  func(Context) string { return "Decoding video on the GPU: normal while watching or calling." },
	},
	{
		level: Info,
		when:  func(c Context) bool { return chromiumRole(c.Details.Cmdline) != "" },
		text: func(c Context) string {
			return "Part of a browser engine (Chromium, Electron or WebView2): this one is " + chromiumRole(c.Details.Cmdline) + "."
		},
	},
	{
		level: Info,
		when:  func(c Context) bool { return c.SameName >= sameNameMany },
		text: func(c Context) string {
			why := "This program runs as several processes."
			if chromiumRole(c.Details.Cmdline) != "" {
				why = "Browser engines (Chromium, Electron, WebView2) split each tab or window into its own process."
			}
			return fmt.Sprintf("One of %d %s processes. %s Press z to group them and see the total.",
				c.SameName, view.GroupName(c.Proc.Name), why)
		},
	},
	{
		level: Info,
		when:  func(c Context) bool { return svchostService(c) != "" },
		text:  func(c Context) string { return "Windows service host for: " + svchostService(c) + "." },
	},
	{
		level: Info,
		when: func(c Context) bool {
			return !c.Proc.CPUKnown && !c.Proc.RSSKnown && c.Proc.PID > 4 && !isolated[strings.ToLower(c.Proc.Name)]
		},
		text: func(Context) string {
			return "The OS protects this process: run syscope as administrator (or with sudo) to see its CPU and memory."
		},
	},
	{
		level: Info,
		when:  func(c Context) bool { return knownProcess(c.Proc.Name) != "" },
		text:  func(c Context) string { return knownProcess(c.Proc.Name) },
	},
}

// cores is the process's CPU use in full cores.
func cores(c Context) float64 { return view.CPUValue(c.Proc.CPU, c.NumCPU, true) / 100 }

var llmRuntimes = []string{"ollama", "llama-server", "llama-cli", "llamafile", "lm studio", "lms", "koboldcpp", "vllm", "localai", "jan"}

func isLLMRuntime(name string) bool {
	n := strings.ToLower(view.GroupName(name))
	for _, r := range llmRuntimes {
		if n == r || strings.HasPrefix(n, r+" ") || strings.HasPrefix(n, r+"_") {
			return true
		}
	}
	return false
}

var chromiumTypeRE = regexp.MustCompile(`--type=([a-z-]+)`)

// chromiumRole explains a Chromium child process from its --type flag.
func chromiumRole(cmdline string) string {
	m := chromiumTypeRE.FindStringSubmatch(cmdline)
	if m == nil {
		return ""
	}
	switch m[1] {
	case "renderer":
		return "a renderer, which runs one tab, page or window's content"
	case "gpu-process":
		return "the GPU process, which draws the app's graphics"
	case "utility":
		return "a utility process (network, audio, storage or similar)"
	case "crashpad-handler":
		return "the crash reporter"
	}
	return "a " + m[1] + " process"
}

var svchostServiceRE = regexp.MustCompile(`-s\s+(\S+)`)
var svchostGroupRE = regexp.MustCompile(`-k\s+(\S+)`)

// svchostService names what a Windows svchost.exe hosts, from its command line.
func svchostService(c Context) string {
	if !strings.EqualFold(c.Proc.Name, "svchost.exe") {
		return ""
	}
	if m := svchostServiceRE.FindStringSubmatch(c.Details.Cmdline); m != nil {
		return m[1]
	}
	if m := svchostGroupRE.FindStringSubmatch(c.Details.Cmdline); m != nil {
		return "services in group " + m[1]
	}
	return ""
}

// isolated processes can't be read even as administrator, so the "run as
// admin" hint would be wrong for them; knownProcesses explains them instead.
var isolated = map[string]bool{"secure system": true, "registry": true, "vmmem": true, "vmmemwsl": true}

// knownProcesses explains common system processes users ask about.
var knownProcesses = map[string]string{
	"dwm":                "Desktop Window Manager: draws every window on screen. GPU and VRAM use grow with monitors, resolution and open windows.",
	"msmpeng":            "Microsoft Defender's scanner. High CPU or disk during scans and updates is normal and temporary.",
	"searchindexer":      "Windows Search indexing files. Busy after installs or file changes, then it settles.",
	"memory compression": "Windows compressing RAM instead of paging to disk. Large values mean memory pressure.",
	"system":             "The Windows kernel. Disk and CPU here are drivers and the OS itself.",
	"secure system":      "Windows virtualization-based security (e.g. Credential Guard), isolated from the OS: no tool can read its usage.",
	"registry":           "The Windows registry, held in memory by the kernel.",
	"vmmem":              "Memory and CPU of a virtual machine (WSL 2, Docker or Hyper-V) as seen by Windows.",
	"vmmemwsl":           "Memory and CPU of the WSL 2 virtual machine (Linux and Docker on Windows).",
	"csrss":              "A core Windows process for consoles and windows. Must not be ended.",
	"wmiprvse":           "Windows Management Instrumentation: answers system queries from apps and tools. Bursts are normal.",
	"explorer":           "Windows Explorer: the taskbar, Start menu and file windows.",
	"kswapd0":            "Linux moving memory to swap. Sustained CPU here means memory pressure.",
	"kworker":            "Linux kernel worker threads doing driver and filesystem work.",
	"windowserver":       "macOS's display server: draws every window, like dwm on Windows.",
	"kernel_task":        "macOS kernel. High CPU here is often the OS slowing things down to manage heat.",
}

func knownProcess(name string) string {
	n := strings.ToLower(view.GroupName(name))
	if note, ok := knownProcesses[n]; ok {
		return note
	}
	if strings.HasPrefix(n, "kworker/") {
		return knownProcesses["kworker"]
	}
	return ""
}
