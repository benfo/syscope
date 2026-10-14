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
| CPU % | Normalized to 100% total (Task Manager); `--irix` / `i` for per-core |
| Memory column | RSS (working set on Windows) |
| Kill | Graceful (`k`) + force (`K`), confirm first; never self-elevate |
| `ps` sampling | Two samples, 1s window (`--interval`) |
| Testing | Windows native, Linux via Docker (`golang:1.27`), macOS compile-only |

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
- [ ] **Search** Type-to-search in the Processes tab (user request; key design to agree first)
- [ ] **2c** GPU: usage, VRAM, temperature, power, clocks, throttle reasons (NVML + Windows GPU counters)
- [ ] **2d** Per-process GPU % and VRAM columns

### Later phases
- [ ] Phase 3 — developers / power users
- [ ] Phase 4 — AI / local LLM
- [ ] Phase 5 — gamers
- [ ] Phase 6 — admin parity

## Done

### Phase 2 — Performance parity
- [x] **2a** Tabs (`1`/`2`, Tab) + summary strip; Performance tab with device sidebar, CPU (history graph, P/E core grid via GetLogicalProcessorInformationEx / sysfs) and Memory (graph, composition, commit charge, cache, swap)
- [x] **2b** Physical disks (Active time, read/write, response time, volumes; IOCTLs on Windows, sysfs on Linux) and network adapters (rx/tx, link speed, address; shown when up and physical or carrying traffic, `a` shows all); DISK/NET in summary strip

### Phase 1 — POC
- [x] Metrics sampler (gopsutil), CPU deltas, PID-reuse safe cache, Toolhelp names on Windows
- [x] View helpers: sort, filter, group-by-name, formatting
- [x] Graceful / force kill per OS (WM_CLOSE on Windows, SIGTERM/SIGKILL on Unix)
- [x] `taskman ps` (table / JSON) and `taskman kill` (PID / `--name` / `--all`)
- [x] Interactive dashboard: CPU/MEM header, sortable/filterable/groupable table, kill with confirm
- [x] Verified on Windows and Linux (Docker); macOS compiles
