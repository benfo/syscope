# taskman roadmap

Cross-platform (Windows / Linux / macOS) process manager and performance monitor
for the terminal, in the spirit of Windows Task Manager — aimed at power users,
developers, AI / local-LLM users and gamers.

Workflow: one feature at a time → build → commit → hands-on test → next.
This file is updated in the same commit as each feature.

## Plan

### Decisions
| Topic | Decision |
|---|---|
| Stack | Go + gopsutil v4 + Bubble Tea / Lip Gloss + Cobra |
| Interaction | `taskman` = live TUI; subcommands (`ps`, `kill`, …) for scripting |
| TUI layout | Tabs (`1` Processes, `2` Performance, `3` Ports, …) + one-line summary strip on every tab; inactive tabs shrink to their number when the tab bar is narrow |
| CPU % | Normalized to 100% total (Task Manager); `--per-core` / `c` for per-core |
| Memory column | RSS (working set on Windows) |
| Kill | Graceful (`k`) + force (`K`), confirm first; never self-elevate |
| `ps` sampling | Two samples, 1s window (`--interval`) |
| Testing | Windows native, Linux via Docker (`golang:1.27`), macOS compile-only |
| Keys | Vim style, one keymap table drives footer, `?` help and tests (see Key rules) |
| Help | `?` explains every metric on the current view (what it means, when to worry); new metrics ship with their explanation |

### Key rules
The same rules on every tab; arrows / PgUp / PgDn / Home / End always work too.

| Rule | Keys |
|---|---|
| Move | `j`/`k` down/up · `g`/`G` top/bottom · `Ctrl+d`/`Ctrl+u` half page |
| Wrapping | **Menus wrap, scrolling lists don't** (as in vim: the completion menu and wildmenu cycle, buffers stop). Menus — short lists shown in full: Settings, the theme list, Performance devices — go from first to last and back. Scrolling lists — processes, ports, help — stop at the ends; use `g`/`G` |
| Search | `/` live filter · `Enter` keep and return to list · `Esc` clear |
| Lowercase acts, uppercase is stronger / reverse | `d` end task / `D` force kill · `s` next sort column / `S` reverse |
| Toggles | `z` group (fold) · `c` CPU% mode · `.` show hidden (Performance: hidden adapters; Ports: connections, not just listeners) |
| Tabs | `1`–`9` jump · `Tab` / `Shift+Tab` cycle |
| Everywhere | `?` help · `Esc` cancel / clear · `q` quit · `y`/`n` confirm |
| Settings | `,` opens Settings (one place for every setting; `j`/`k` select, `h`/`l` change with live preview, `Enter` on Theme opens the theme list). Dashboard keys change only the current session. The footer drops hints when narrow but always keeps `? help` |
| Overlays | Never swallow global keys: tab keys work in `?` help (it follows you to the new tab); `q`/`Esc` close an overlay first. Exceptions, as in vim: search input is like insert mode (keys type; `Enter`/`Esc` leave) and y/N treats any other key as no |
| Reserved | `Enter` details (on Ports: go to the owning process) · `t` tree · `p` suspend/resume · `+`/`-` priority · `o` open file location |

New features must fit these rules and be added to the keymap table, never bound ad hoc.

### Code standards
The codebase must stay easy to maintain:
- **DRY**: one helper per idea (formatting lives in `view`, layout helpers in `tui/widgets.go`); no copy-paste variants.
- **SOLID**: small interfaces over growing `switch`es, so a new device / collector / column is a new file, not edits across many; one responsibility per type and file.
- **Naming**: code names match the concept users see (e.g. "per-core CPU%", not internal jargon).
- **Tests**: behaviour covered by unit tests; golden screen tests (`internal/tui/testdata`) catch any visual change; refactors land in their own commits with behaviour unchanged.
- **Platforms**: OS-specific code in `*_windows.go` / `*_linux.go` / `*_other.go` files grouped per concern.
- **Privacy**: the repo is public. Test fixtures, golden screens, examples, comments and docs use **synthetic data only** — fictional names, `example.com` emails, documentation IPs (192.0.2.x, 198.51.100.x, 203.0.113.x, 2001:db8::), generic apps and hardware. Never copy values from a real machine (user names, IPs, running apps, exact hardware). `internal/repocheck` fails the build on real-looking IPs or emails; names need review.

### Phases
1. **POC** — process table, CPU/MEM header, `ps`, `kill`
2. **Performance parity** — tabs, CPU core grid (P/E), memory, disk, network, GPU, per-process GPU/VRAM
3. **Developers / power users** — process tree, details pane, ports view, kill tree, suspend/resume, priority, config file
4. **AI / local LLM** — detect Ollama / LM Studio / llama.cpp / vLLM / ComfyUI, loaded models + VRAM, headroom
5. **Gamers** — game mode (suspend / deprioritize background hogs); FPS via PresentMon (stretch)
6. **Admin parity** — services, startup apps, users

Stretch: `taskman record` (CSV/JSON logging), `taskman serve` (Prometheus endpoint).
Out of scope: per-process network rates, app history.

## To do

### Phase 2 — Performance parity
- [ ] **GPU: Linux Intel** (i915/xe sysfs: clock, power; usage needs root) and **macOS Apple Silicon** (IOKit performance statistics via purego)


### Theming (user request, 2026-10-14)
- [ ] **Themes, part 2** — user themes from a file in the config dir and community themes (shareable files; maybe a `themes/` folder in the repo); optional painting of the background colour (themes currently colour text only). Depends on **Preferences**.

### Investigate (user ideas; check viability on all OSes before committing to a design)
- [ ] **Apps vs Background processes** — split the list into Apps / Background (/ OS processes) like Task Manager. First look: Windows: viable (apps own a visible top-level window — same enumeration `d` already uses; Windows processes = session 0). Linux: no universal notion; likely viable on systemd desktops (GNOME/KDE launch apps into `app-*.scope` cgroups), fallback heuristic otherwise. macOS: viable via NSRunningApplication activation policy (Objective-C runtime through purego, no cgo). Needs a spike per OS. (2026-10-14)
- [ ] **Focus an app from the process list** (user idea) — on an app (a process with a window, see Apps vs Background), a key brings its window to the front, like Task Manager's "Switch to". Key: `Enter` is reserved for "details" in Key rules, so either `Enter` = focus for apps / details for background processes (context-dependent, less consistent), or a dedicated key (e.g. `f` focus) with `Enter` always = details (recommended for consistency). First look per OS: Windows viable (EnumWindows by PID, as end task already does, then SetForegroundWindow; Windows blocks focus-stealing from background processes, so it needs the usual AllowSetForegroundWindow/AttachThreadInput workaround — needs a spike). macOS viable (NSRunningApplication activate via Objective-C runtime through purego). Linux X11 viable (_NET_ACTIVE_WINDOW); Wayland mostly not (no standard protocol; GNOME/KDE only via their D-Bus APIs) — degrade with a message. Depends on Apps vs Background. (2026-10-14)
- [ ] **"Why" hints, part 2** — the remaining rule from the original idea: **memory growing steadily = possible leak** (needs per-process memory history; pairs with Adjustable history timeframe). More known-process notes as users ask. (Part 1 done.)
- [ ] **Local anomaly detection (machine learning, no LLM)** (user idea, 2026-10-14) — a different kind of AI: learn what's *normal for this machine* and flag what isn't, entirely offline and tiny. Ideas: per-process and per-device baselines (rolling mean/variance, time-of-day aware) persisted in the config dir; flags like "chrome is using 3× its usual memory", "disk active time unusual for this hour"; leak detection as a sustained upward trend (regression slope / Mann–Kendall) — replaces the "Why hints part 2" leak rule; change-point detection for "this started 10 minutes ago". Microseconds per sample, explainable, no model download. Later option: a small shipped classifier (KBs of weights) for process categories. Needs longer history (see Adjustable history timeframe).
- [ ] **AI explanations** (user idea, 2026-10-14) — ask an AI to explain *what you're looking at*: the selected process (with its hints and details), a Performance device page, or the whole screen ("why is my laptop slow right now?"). Ideas: a key (e.g. `a` ask) opens a pane with a streamed answer and follow-up questions; providers: Claude (API key) or a **local model** via Ollama / LM Studio (OpenAI-compatible endpoint) so nothing leaves the machine; **privacy first** — opt-in, show exactly what will be sent, redact user names and paths by default; the rule-based hints stay the free, offline baseline. Open questions: key, default provider, cost display, where the API key lives (Preferences / OS keychain). (Merges the earlier "ask AI" layer.) Model options noted 2026-10-14: small local LLMs (1–4 B parameters, e.g. via Ollama) are enough for explanations; **Jev** (TypeSafe AI, early access Sept 2026) is a proprietary cloud *decision* model — returns classifications with confidence scores, not text — so a possible optional classifier, but not local.
- [ ] **High-resolution graphs** (user idea) — today graphs use eighth-blocks: 1 sample per column, 8 levels per row. Options: (a) **Braille** characters (2×4 dots per cell, as btop does) — double the time resolution and smooth line graphs, works in any Unicode terminal; (b) **terminal graphics protocols** (Sixel — supported by Windows Terminal 1.22+, Kitty, iTerm2, WezTerm) for real pixel graphs, with Braille/blocks as fallback when unsupported. Probably (a) first, (b) as an opt-in. Could be a preference. (2026-10-14)
- [ ] **Adjustable history timeframe** (user idea) — graphs keep 300 samples (5 min at 1 s refresh). Ideas: a setting / `--history 30m`; zoom keys on the Performance tab to view 1 min … 24 h (not `+`/`-`, reserved for priority — e.g. `[` / `]`); keep long history cheaply with tiered downsampling (1 s for 5 min, 10 s for 1 h, 1 min for 24 h, keeping min/max/avg per bucket so spikes aren't lost). Ties in with Preferences and the `taskman record` stretch goal. (2026-10-14)
- [ ] **Enter on a Performance device** (user question) — `Enter` is reserved for "details", so it should drill into the selected device. Options: (a) **"who's using it"**: jump to Processes filtered/sorted by that resource — CPU→CPU%, Memory→MEM, GPU→GPU%/VRAM (needs 2d), Disk→disk I/O, Network→connections (Phase 3); (b) **full-screen device view**: bigger graph plus extra detail (per-core graphs, per-volume I/O, per-engine GPU graphs); (c) both: `Enter` opens the full-screen view, and a key there jumps to the processes using it. Decide after 2d, when per-process GPU exists. (2026-10-14)

### Revisit (Claude reminds at the trigger)
- [ ] **Help on Performance can't change device** — while `?` is open, `j`/`k` scroll the help, so you must close it to explain another device. Option: help as a side panel that follows the selection. *Trigger: end of Phase 2 (after 2d), once help has had real use.* (noted 2026-10-14)

### Later phases
- [ ] Phase 3 — developers / power users
- [ ] Phase 4 — AI / local LLM
- [ ] Phase 5 — gamers
- [ ] Phase 6 — admin parity

## Done

### Phase 2 — Performance parity
- [x] **2a** Tabs (`1`/`2`, Tab) + summary strip; Performance tab with device sidebar, CPU (history graph, P/E core grid via GetLogicalProcessorInformationEx / sysfs) and Memory (graph, composition, commit charge, cache, swap)
- [x] **2b** Physical disks (Active time, read/write, response time, volumes; IOCTLs on Windows, sysfs on Linux) and network adapters (rx/tx, link speed, address; shown when up and physical or carrying traffic, `a` shows all); DISK/NET in summary strip
- [x] **Keys** Vim-style keymap per Key rules (replaces type-to-search request): single keymap table drives footer, `?` help overlay and consistency tests; rebound k→d, g→z, i→c, a→.
- [x] **Explain** `?` opens "About this view": context-aware explanations (process columns; CPU, memory, disk, network metrics, only those shown on this platform) above the key list; `j`/`k` scroll, `Esc` closes
- [x] **2c** GPU page: every GPU on Windows (DXGI + Task Manager's PDH counters: per-engine usage, dedicated/shared memory; D3DKMT temperature/fan/power for any vendor), NVIDIA deep stats via pure-Go NVML (temp, power vs limit, clocks, P-state, throttle reasons) on Windows + Linux, Linux AMD via amdgpu sysfs; GPU + VRAM in top line, which now drops low-priority items when narrow; stat lines wrap
- [x] **Refactor** to Code standards (behaviour unchanged, proven by golden screen tests + colour comparison): formatting helpers in `view`, shared grid/stat layout; irix→per-core (`--irix` kept as deprecated alias), ring→history, cpu→cpuInfo; all colours behind a `Theme` of named roles; Performance devices behind a `devicePage` interface (one `page_*.go` per device); Processes tab extracted to `processTable`; sampler split into per-area collectors; metrics files and tests grouped per concern
- [x] **2d** Per-process GPU% (busiest engine) and VRAM: Processes columns (only on machines with a GPU; sortable; summed in groups; explained in `?`) and in `taskman ps` (`--sort gpu|vram`, JSON `gpu_percent`/`vram_bytes`). Sources: Windows per-process GPU counters (any vendor), NVML process lists (NVIDIA), DRM fdinfo (Linux AMD/Intel). Idle processes show 0 when the source covers every process, `--` only when genuinely unknown. Processes columns are now a table (`processColumns`), and collectors can enrich processes (`procEnricher`)
- [x] **Themes, part 1** — 10 built-in presets (default, dracula, catppuccin-mocha, tokyo-night, nord, gruvbox-dark, solarized-dark, github-light, high-contrast, colorblind / Okabe–Ito), `--theme`, `taskman themes` preview, `T` live-preview picker popover
- [x] **Details pane** (Phase 3, part 1) — `Enter` opens a bottom split following the cursor: path, command line, parent/children, user, start time, threads, handles, live I/O rate, CPU/memory/GPU (which GPU and engine)/VRAM; group summary for grouped rows; `Esc` closes before clearing search
- [x] **"Why" hints, part 1** — `internal/insight` rule table shown in the details pane (ℹ explains, ⚠ act): idle cached VRAM, LLM fell back to CPU, big RAM share, video encode/decode, browser-engine child roles, many processes, svchost service names, protected processes (accurate for isolated ones like Secure System), notes on well-known system processes. Checked against real processes on Windows
- [x] **Preferences** — auto-saved TOML in the OS config dir (`taskman config path`): theme, refresh, last tab, sort/direction, grouping, per-core CPU%, details pane, show-hidden devices. Only changed settings are saved (one-run flag overrides aren't); background atomic saves plus a final save on quit; a broken file starts on defaults without overwriting it; `taskman config show|path|reset`, `--config <file>`. `taskman ps` deliberately ignores preferences
- [x] **Process tree + expandable groups** (Phase 3) — layouts flat / grouped (`z`) / tree (`t`); `l`/`h` (or →/←) expand/collapse or go to parent, `L`/`H` all; groups start closed, the tree open; search keeps matches' ancestors; processes in parent cycles are never hidden; group rows read `chrome (31)`; layout saved as a preference
- [x] **Settings screen + what persists** — `,` opens Settings (Theme, Refresh, CPU%, Default layout, Default sort, Restore last session), saved as you change them; `T` removed. Only settings persist; dashboard keys are session-only unless "Restore last session" is on, which saves and restores tab, sort, layout, CPU% mode, details pane and hidden devices in a `[session]` section. Footer always keeps `? help`
- [x] **Wrap-around in menus** — Settings, theme list and Performance devices wrap; processes and help don't (one shared `menuMove` helper; rule in Key rules)
- [x] **Per-volume disks** — volumes of a multi-volume disk appear indented under it in the Performance sidebar with their own read/write graphs and usage (Windows per-volume IOCTL counters, Linux partition counters; macOS and virtual drives show usage only, and say so). Active and response time stay on the physical disk's page. Nested sidebar entries via an optional `nested` interface. Virtual drives (Windows drive letters that aren't `\Device\HarddiskVolume…` partitions, e.g. cloud-sync drives) are kept off physical disks and named in a note on the disk page
- [x] **Ports view** (Phase 3) — tab `3`: listening sockets (TCP LISTEN and bound UDP) with port, protocol, address and owning process; `.` adds every connection with remote end and state; `/` searches port, name, PID or address; `s`/`S` sort; `d`/`D` end the owner (with y/N); `Enter` jumps to the owner on Processes with its details open (clearing the search or opening its group/tree ancestors when they hide it); `?` explains 0.0.0.0 vs 127.0.0.1 and unknown owners (Linux needs root for other users' sockets). Sockets via gopsutil on every OS (~2 ms per sample). The Processes table's scrolling, columns, search box and kill prompt became shared pieces (`listnav.go`, `kill.go`)

### Phase 1 — POC
- [x] Metrics sampler (gopsutil), CPU deltas, PID-reuse safe cache, Toolhelp names on Windows
- [x] View helpers: sort, filter, group-by-name, formatting
- [x] Graceful / force kill per OS (WM_CLOSE on Windows, SIGTERM/SIGKILL on Unix)
- [x] `taskman ps` (table / JSON) and `taskman kill` (PID / `--name` / `--all`)
- [x] Interactive dashboard: CPU/MEM header, sortable/filterable/groupable table, kill with confirm
- [x] Verified on Windows and Linux (Docker); macOS compiles
