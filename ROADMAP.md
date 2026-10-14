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
| TUI layout | Tabs (`1` Processes, `2` Performance, …) + one-line summary strip on every tab |
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
| Search | `/` live filter · `Enter` keep and return to list · `Esc` clear |
| Lowercase acts, uppercase is stronger / reverse | `d` end task / `D` force kill · `s` next sort column / `S` reverse |
| Toggles | `z` group (fold) · `c` CPU% mode · `.` show hidden |
| Tabs | `1`–`9` jump · `Tab` / `Shift+Tab` cycle |
| Everywhere | `?` help · `Esc` cancel / clear · `q` quit · `y`/`n` confirm |
| Overlays | Never swallow global keys: tab keys work in `?` help (it follows you to the new tab); `q`/`Esc` close an overlay first. Exceptions, as in vim: search input is like insert mode (keys type; `Enter`/`Esc` leave) and y/N treats any other key as no |
| Reserved | `Enter` details · `t` tree · `p` suspend/resume · `+`/`-` priority · `o` open file location |

New features must fit these rules and be added to the keymap table, never bound ad hoc.

### Code standards
The codebase must stay easy to maintain:
- **DRY**: one helper per idea (formatting lives in `view`, layout helpers in `tui/widgets.go`); no copy-paste variants.
- **SOLID**: small interfaces over growing `switch`es, so a new device / collector / column is a new file, not edits across many; one responsibility per type and file.
- **Naming**: code names match the concept users see (e.g. "per-core CPU%", not internal jargon).
- **Tests**: behaviour covered by unit tests; golden screen tests (`internal/tui/testdata`) catch any visual change; refactors land in their own commits with behaviour unchanged.
- **Platforms**: OS-specific code in `*_windows.go` / `*_linux.go` / `*_other.go` files grouped per concern.

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
- [ ] **Refactor** apply Code standards: colours behind one Theme type (prepares theming), device pages behind an interface, sampler split into collectors, DRY formatting/grids, naming (irix→per-core, ring→history), process-tab state out of model, per-concern platform files and tests
- [ ] **2d** Per-process GPU % and VRAM columns
- [ ] **GPU: Linux Intel** (i915/xe sysfs: clock, power; usage needs root) and **macOS Apple Silicon** (IOKit performance statistics via purego)

### Theming (user request, 2026-10-14)
- [ ] **Themes** — built-in presets (e.g. default, light, high-contrast, colour-blind-safe, popular palettes like Dracula / Catppuccin / Gruvbox), user themes from a file in the config dir, and community themes (shareable files; maybe a themes folder in the repo). Prerequisite (in Refactor): all colours behind one `Theme` type of named roles. Open questions: file format (TOML/JSON), switching keys/flag (`--theme`), terminal colour support (16 / 256 / truecolor fallbacks).

### Investigate (user ideas; check viability on all OSes before committing to a design)
- [ ] **Expandable groups** — in group mode (`z`), expand a group to see its individual processes, like Task Manager. First look: pure UI, the data is already there, so viable on every OS. Open questions: keys under the fold rules (e.g. `l`/`h` or `Enter` to expand/collapse one group, `zR`/`zM` all), and how it relates to the Phase 3 tree view (`t`). (2026-10-14)
- [ ] **Apps vs Background processes** — split the list into Apps / Background (/ OS processes) like Task Manager. First look: Windows: viable (apps own a visible top-level window — same enumeration `d` already uses; Windows processes = session 0). Linux: no universal notion; likely viable on systemd desktops (GNOME/KDE launch apps into `app-*.scope` cgroups), fallback heuristic otherwise. macOS: viable via NSRunningApplication activation policy (Objective-C runtime through purego, no cgo). Needs a spike per OS. (2026-10-14)

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

### Phase 1 — POC
- [x] Metrics sampler (gopsutil), CPU deltas, PID-reuse safe cache, Toolhelp names on Windows
- [x] View helpers: sort, filter, group-by-name, formatting
- [x] Graceful / force kill per OS (WM_CLOSE on Windows, SIGTERM/SIGKILL on Unix)
- [x] `taskman ps` (table / JSON) and `taskman kill` (PID / `--name` / `--all`)
- [x] Interactive dashboard: CPU/MEM header, sortable/filterable/groupable table, kill with confirm
- [x] Verified on Windows and Linux (Docker); macOS compiles
