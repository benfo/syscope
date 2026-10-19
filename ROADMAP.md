# syscope roadmap

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
| Interaction | `syscope` = live TUI; subcommands (`ps`, `kill`, …) for scripting |
| TUI layout | Tabs (`1` Processes, `2` Performance, `3` Ports, …) + one-line summary strip on every tab; inactive tabs shrink to their number when the tab bar is narrow |
| CPU % | Normalized to 100% total (Task Manager); `--per-core` / `c` for per-core |
| Memory column | RSS (working set on Windows) |
| Name | **syscope** (renamed from taskman, 2026-10-15): "a scope for your system". Module `github.com/benfo/syscope`; repo github.com/benfo/syscope (public since 2026-10-19) |
| Kill | Graceful (`d`) + force (`D`), confirm first (`t` adds the processes it started); never self-elevate |
| Suspend | `p` asks before suspending, resumes without asking; never suspends syscope or its terminal; quitting offers to resume what this session suspended |
| `ps` sampling | Two samples, 1s window (`--interval`) |
| Testing | Windows native, Linux via Docker (`golang:1.27`), macOS compile-only (see How to test) |
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
| Toggles | `z` group (fold) · `c` CPU% mode · `p` suspend (asks) / resume · `.` show hidden (Performance: hidden adapters; Ports: connections, not just listeners) |
| Tabs | `1`–`9` jump · `Tab` / `Shift+Tab` cycle |
| Everywhere | `?` help · `Esc` cancel / clear · `q` quit · `y`/`n` confirm (kill prompts add `t`: also the processes it started) |
| Settings | `,` opens Settings (one place for every setting; `j`/`k` select, `h`/`l` change with live preview, `Enter` on Theme opens the theme list). Dashboard keys change only the current session. The footer drops hints when narrow but always keeps `? help` |
| Overlays | Never swallow global keys: tab keys work in `?` help (it follows you to the new tab); `q`/`Esc` close an overlay first. Exceptions, as in vim: search input is like insert mode (keys type; `Enter`/`Esc` leave) and y/N treats any other key as no |
| Reserved | `Enter` details (on Ports: go to the owning process) · `t` tree · `+`/`-` priority · `o` open file location |

New features must fit these rules and be added to the keymap table, never bound ad hoc.

### Code standards
The codebase must stay easy to maintain:
- **DRY**: one helper per idea (formatting lives in `view`, layout helpers in `tui/widgets.go`); no copy-paste variants.
- **SOLID**: small interfaces over growing `switch`es, so a new device / collector / column is a new file, not edits across many; one responsibility per type and file.
- **Naming**: code names match the concept users see (e.g. "per-core CPU%", not internal jargon).
- **Tests**: behaviour covered by unit tests; golden screen tests (`internal/tui/testdata`) catch any visual change; refactors land in their own commits with behaviour unchanged.
- **Platforms**: OS-specific code in `*_windows.go` / `*_linux.go` / `*_other.go` files grouped per concern.
- **Privacy**: the repo is public. Test fixtures, golden screens, examples, comments and docs use **synthetic data only** — fictional names, `example.com` emails, documentation IPs (192.0.2.x, 198.51.100.x, 203.0.113.x, 2001:db8::), generic apps and hardware. Never copy values from a real machine (user names, IPs, running apps, exact hardware). `internal/repocheck` fails the build on real-looking IPs or emails; names need review.

### How to test
- **Everything, Windows:** `go vet ./... && go test ./...`, then `go install ./cmd/syscope`
- **Linux (Docker):** `docker run --rm -v "$PWD:/src" -v syscope-gomod:/go/pkg/mod -w /src golang:1.27 sh -c 'go vet ./... && go test ./...'`
- **macOS:** compile-only, `GOOS=darwin go vet ./...`
- **Screens:** golden tests in `internal/tui/testdata`; after an intended visual change, `go test ./internal/tui -run Golden -update` and review the diff
- **README screenshots:** `go test ./internal/tui -run Screenshots -screenshots` renders `docs/screenshots/*.svg` from synthetic data (`internal/tui/screenshot_test.go`); rerun after visible changes
- **Try Linux by hand (Docker):** builds syscope in a container with demo listeners (TCP 8080, UDP 5353, a connection) and two `bash` processes that each started two `sleep`s. Use `--init`; without it syscope is PID 1, nothing reaps exited processes, and they stay listed as `(exited)` (handy for testing that):
  `docker run --rm -it --init -v "$PWD:/src" -v syscope-gomod:/go/pkg/mod -w /src golang:1.27 bash -c 'apt-get update -qq >/dev/null && apt-get install -y -qq netcat-openbsd >/dev/null && go build -o /usr/local/bin/syscope ./cmd/syscope && (nc -lk 8080 >/dev/null &) && (nc -lku 5353 >/dev/null &) && (sleep 9999 | nc localhost 8080 &) && (bash -c "sleep 9999 & sleep 9999 & wait" &) && (bash -c "sleep 9999 & sleep 9999 & wait" &) && sleep 1 && syscope'`
  Replace the final `syscope` with `useradd -m demo && su demo -c syscope` to see Linux without root (socket owners `(unknown)`).
- **Docker's limits:** only the container's processes; CPU/memory/disk/network are Docker Desktop's VM; no GPU.
- **Release a test build:** `scripts/release.sh v0.x.y` builds all six OS/CPU archives into `dist/` (neutral file owner, executable bit set, `--version` stamped), then `gh release create v0.x.y --prerelease --target master dist/syscope_* dist/SHA256SUMS`. Users install with `install.sh` / `install.ps1` (see README), which fetch the newest release (pre-releases too while there is no full release) and check `SHA256SUMS`; the README links them from `master`, so they must keep working there. v0.1.0 released 2026-10-15

### Phases
1. **POC** — process table, CPU/MEM header, `ps`, `kill` ✓
2. **Performance parity** — tabs, CPU core grid (P/E), memory, disk, network, GPU, per-process GPU/VRAM ✓ (Linux Intel / macOS GPU left)
3. **Developers / power users** — process tree ✓, details pane ✓, ports view ✓, kill tree ✓, suspend/resume ✓, priority, config file ✓
4. **AI / local LLM** — detect Ollama / LM Studio / llama.cpp / vLLM / ComfyUI, loaded models + VRAM, headroom
5. **Gamers** — game mode (suspend / deprioritize background hogs); FPS via PresentMon (stretch)
6. **Admin parity** — services, startup apps, users

Stretch: `syscope record` (CSV/JSON logging), `syscope serve` (Prometheus endpoint).
Out of scope: per-process network rates, app history.

## To do

### Next up (where we left off, 2026-10-15)
Chosen by the user on 2026-10-15, in this order:
1. [ ] **Priority (`+`/`-`) + efficiency mode + "Not responding"** — last Phase 3 item; the keys are reserved. Windows priority classes (Idle … Realtime; never offer Realtime), Unix nice (−20…19). Lowering works for your own processes; raising needs admin / root, so errors must say so. Design questions to ask first: a PRI column or details-pane only? confirm before raising? what `+`/`-` do on a group.
   - **Efficiency mode** (Windows 11 EcoQoS: `SetProcessInformation(ProcessPowerThrottling)` + Idle priority, as Task Manager does) — shown like `(suspended)` (e.g. a leaf/`eco` marker) and toggled by a key (ask: `e`?). On Unix there is no equivalent; either map to nice 19 or hide the action — decide with the user.
   - **"Not responding"** — Windows: a process whose top-level window is hung (`IsHungAppWindow`, reusing the window enumeration `d` uses) shows `(not responding)` with a details hint, and is a good `D` candidate. macOS: possible later (no public API; skip for now). Linux: no standard notion — don't show.
2. [x] **Braille graphs** ✓ 2026-10-15 — see Done
3. [ ] **History & rewind** — see the plan below (adjustable timeframe, then rewind)
4. [ ] **Which process has this file locked?** — the classic "can't delete / file in use" fix (Process Explorer's Find Handle). Ideas: `syscope who-locks <path>` CLI plus a dashboard search (key to decide, e.g. on Processes `/` with a path prefix, or a dedicated prompt); also list a process's open files in the details pane. Per OS: Windows — Restart Manager API (`RmGetList`, the same one installers use; no admin, fast) for files, fallback handle enumeration (`NtQuerySystemInformation` handle table + `NtQueryObject`, needs care: some handles hang, query on a worker with timeout); Linux — scan `/proc/*/fd` symlinks (other users' processes need root, say so); macOS — `proc_pidinfo` fd lists via purego (spike). Directories count too (a shell `cd`'d into a folder blocks deleting it on Windows)
- [ ] **`syscope kill --tree`** (optional) — the CLI equivalent of the `t` answer in the dashboard's kill prompt

### History & graphs (plan, 2026-10-15)
Merges the earlier "High-resolution graphs" and "Adjustable history timeframe" ideas, plus rewind. Build in steps, each shippable:
1. **Braille graphs** ✓ (2026-10-15).
2. **Long history store** — graphs keep 300 samples (5 min at 1 s) today. Tiered downsampling: 1 s for 5 min, 10 s for 1 h, 1 min for 24 h, keeping min/max/avg per bucket so spikes aren't lost. Setting / `--history 30m`; zoom on Performance with `[` / `]` (not `+`/`-`, priority). Memory budget to check (devices × metrics × buckets is small; per-process is the expensive part).
3. **Per-process history** — small CPU / memory graphs for the selected process in the details pane; keep a compact history for the top-N processes per sample (not every process), so cost stays bounded. Enables leak detection ("Why" hints part 2 / anomaly detection).
4. **Rewind** — move back through history and see the **process list as it was** at that moment ("what was eating CPU at 03:12?"). Needs per-sample process snapshots (top-N by CPU / memory / GPU / disk, stored at the same tiers). UI to design: a cursor on the Performance graph (`h`/`l` moves in time?) with a "⏪ 03:12:40 — live in `Esc`" banner, and the Processes table showing that moment read-only (no kill / suspend). Spike markers on graphs; a key jumps to the next spike.
5. **Persist across restarts** (optional) — a background recorder writing a rolling file in the data dir (size-capped), so history survives closing syscope / reboots; shares its format with the `syscope record` stretch goal.
6. **Pixel graphs** (opt-in, later) — Sixel / Kitty graphics where the terminal supports them, Braille fallback.

### Before making the repo public
- [ ] **README** — what it is, a screenshot made from the synthetic golden data (never a real machine), install (`go install github.com/benfo/syscope/cmd/syscope@latest`), keys overview, platforms
- [ ] **License** — choose one (ask the user; MIT or Apache-2.0 are the usual picks)
- [ ] **Delete `backup/pre-scrub`** (local only, contains old personal data): `git branch -D backup/pre-scrub && git reflog expire --expire=now --all && git gc --prune=now`. Never push it
- [ ] **CI** (suggestion) — GitHub Actions running `go vet` + `go test` on Windows, Linux and **macOS runners**, which would finally test macOS for real instead of compile-only
- [ ] **Signed releases** (later) — v0.1.0 binaries are unsigned (Windows SmartScreen and macOS Gatekeeper warn; the release notes explain the workaround). Signing needs certificates (Windows code signing, Apple Developer ID). A Homebrew tap and a Scoop bucket don't need signing (next install step if wanted); winget is easier with it
- [ ] **Default branch** — repo uses `master`; the README's install one-liners point at `master`, so a rename to `main` must update them (and old links may break)
- [x] **Go public** (2026-10-19). History was checked on 2026-10-15 (every commit uses the intended public author identity; three old comments mention Google Drive's default drive letter, judged fine)
- [ ] **musl Linux build** (suggestion) — the Linux build is dynamically linked (purego loads NVML), so Alpine needs `apk add gcompat`; the installer says so. A static musl variant would need NVML left out or loaded differently

### Testing setup (suggestions, 2026-10-15)
- [ ] **WSL2 Ubuntu** for everyday Linux testing on the dev machine: a full system (many processes, users, permissions), and probably NVIDIA via WSL's driver passthrough — the first real test of the Linux NVML path. `wsl --install -d Ubuntu`, then install Go
- [ ] **Ubuntu live USB at milestones** — the only real test of Linux hardware paths (NVMe sysfs, Wi-Fi, Intel/NVIDIA GPU, P/E core grid); needed for "GPU: Linux Intel"

### Polish
- [ ] **Footer hint priority** — with `p suspend` added, `1-3 tabs` no longer fits at 100 columns (hints drop from the end). Option: give hints a priority so tab keys outlast rarer ones
- [ ] **Faster first screen** (offered 2026-10-14, not requested yet) — start+exit ~60 ms, but the first full sample takes ~300 ms (process list ~0.35 s, CPU% needs a second sample ~0.7 s). Options: show the table as soon as names are known, fill numbers in after

### Phase 2 — Performance parity
- [ ] **GPU: Linux Intel** (i915/xe sysfs: clock, power; usage needs root) and **macOS Apple Silicon** (IOKit performance statistics via purego)

### Ports follow-ups (2026-10-15)
- [ ] **Merge dual-stack listeners** — on Windows a dual-stack listener shows as two rows (`0.0.0.0` and `::`, e.g. port 445), as in `netstat`. Option: one row with address "all (IPv4+IPv6)" when proto, port and PID match
- [ ] **Remember show-all** — `.` on Ports isn't part of the restored session (`[session]`); add it alongside `show_hidden`

### Theming (user request, 2026-10-14)
- [ ] **Themes, part 2** — user themes from a file in the config dir and community themes (shareable files; maybe a `themes/` folder in the repo); optional painting of the background colour (themes currently colour text only). Depends on **Preferences**.

### Investigate (user ideas; check viability on all OSes before committing to a design)
- [ ] **Apps vs Background processes** — split the list into Apps / Background (/ OS processes) like Task Manager. First look: Windows: viable (apps own a visible top-level window — same enumeration `d` already uses; Windows processes = session 0). Linux: no universal notion; likely viable on systemd desktops (GNOME/KDE launch apps into `app-*.scope` cgroups), fallback heuristic otherwise. macOS: viable via NSRunningApplication activation policy (Objective-C runtime through purego, no cgo). Needs a spike per OS. (2026-10-14)
- [ ] **Focus an app from the process list** (user idea) — on an app (a process with a window, see Apps vs Background), a key brings its window to the front, like Task Manager's "Switch to". Key: `Enter` is reserved for "details" in Key rules, so either `Enter` = focus for apps / details for background processes (context-dependent, less consistent), or a dedicated key (e.g. `f` focus) with `Enter` always = details (recommended for consistency). First look per OS: Windows viable (EnumWindows by PID, as end task already does, then SetForegroundWindow; Windows blocks focus-stealing from background processes, so it needs the usual AllowSetForegroundWindow/AttachThreadInput workaround — needs a spike). macOS viable (NSRunningApplication activate via Objective-C runtime through purego). Linux X11 viable (_NET_ACTIVE_WINDOW); Wayland mostly not (no standard protocol; GNOME/KDE only via their D-Bus APIs) — degrade with a message. Depends on Apps vs Background. (2026-10-14)
- [ ] **"Why" hints, part 2** — the remaining rule from the original idea: **memory growing steadily = possible leak** (needs per-process memory history; pairs with per-process history in History & graphs). More known-process notes as users ask. (Part 1 done.)
- [ ] **Local anomaly detection (machine learning, no LLM)** (user idea, 2026-10-14) — a different kind of AI: learn what's *normal for this machine* and flag what isn't, entirely offline and tiny. Ideas: per-process and per-device baselines (rolling mean/variance, time-of-day aware) persisted in the config dir; flags like "chrome is using 3× its usual memory", "disk active time unusual for this hour"; leak detection as a sustained upward trend (regression slope / Mann–Kendall) — replaces the "Why hints part 2" leak rule; change-point detection for "this started 10 minutes ago". Microseconds per sample, explainable, no model download. Later option: a small shipped classifier (KBs of weights) for process categories. Needs longer history (see History & graphs).
- [ ] **AI explanations** (user idea, 2026-10-14) — ask an AI to explain *what you're looking at*: the selected process (with its hints and details), a Performance device page, or the whole screen ("why is my laptop slow right now?"). Ideas: a key (e.g. `a` ask) opens a pane with a streamed answer and follow-up questions; providers: Claude (API key) or a **local model** via Ollama / LM Studio (OpenAI-compatible endpoint) so nothing leaves the machine; **privacy first** — opt-in, show exactly what will be sent, redact user names and paths by default; the rule-based hints stay the free, offline baseline. Open questions: key, default provider, cost display, where the API key lives (Preferences / OS keychain). (Merges the earlier "ask AI" layer.) Model options noted 2026-10-14: small local LLMs (1–4 B parameters, e.g. via Ollama) are enough for explanations; **Jev** (TypeSafe AI, early access Sept 2026) is a proprietary cloud *decision* model — returns classifications with confidence scores, not text — so a possible optional classifier, but not local.
- [ ] **More ideas from the 2026-10-15 brainstorm** (not chosen yet): CPU affinity; restart a process (same command line); watch / alerts ("notify when X exits", "CPU > 90% for 1 min" — desktop notification or bell); process start/exit log (catches short-lived processes); pin / watch list; details-pane extras (environment, loaded DLLs / libraries, threads); CPU / motherboard temperatures and fans (Linux hwmon; Windows hard); battery drain and per-process energy; per-process disk I/O column; memory pressure (page faults/s, compression, heavy swapping warning); container view (group by Docker / Podman container with totals)
- [ ] **Enter on a Performance device** (user question) — `Enter` is reserved for "details", so it should drill into the selected device. Options: (a) **"who's using it"**: jump to Processes filtered/sorted by that resource — CPU→CPU%, Memory→MEM, GPU→GPU%/VRAM (needs 2d), Disk→disk I/O, Network→connections (Phase 3); (b) **full-screen device view**: bigger graph plus extra detail (per-core graphs, per-volume I/O, per-engine GPU graphs); (c) both: `Enter` opens the full-screen view, and a key there jumps to the processes using it. Decide after 2d, when per-process GPU exists. (2026-10-14)

### Revisit (Claude reminds at the trigger)
- [ ] **Help on Performance can't change device** — while `?` is open, `j`/`k` scroll the help, so you must close it to explain another device. Options: fix it (help as a bottom pane that follows the selection, like the details pane), keep as is, or drop. *Trigger reached (end of Phase 2); raised 2026-10-14 and 2026-10-15 — **awaiting the user's decision**.* (noted 2026-10-14)

### Later phases
- [ ] Phase 3 — developers / power users (only priority left)
- [ ] Phase 4 — AI / local LLM
- [ ] Phase 5 — gamers
- [ ] Phase 6 — admin parity

## Done

### History & graphs
- [x] **Braille graphs** (2026-10-15) — Performance graphs draw with Braille (2×4 dots per character): twice the samples per column of the eighth-block graphs, so a graph shows twice the time (spans in titles count it). New **Graphs** setting (`braille` default / `blocks` fallback for fonts without Braille; `graphs` in preferences); graph styles are a small table in `tui/graphs.go`, ready for pixel graphs later. Graphs keep 600 samples (was 300) to fill wide terminals. Sidebar and summary-strip sparklines stay blocks: one row of Braille has only 4 levels against 8

### Phase 2 — Performance parity
- [x] **2a** Tabs (`1`/`2`, Tab) + summary strip; Performance tab with device sidebar, CPU (history graph, P/E core grid via GetLogicalProcessorInformationEx / sysfs) and Memory (graph, composition, commit charge, cache, swap)
- [x] **2b** Physical disks (Active time, read/write, response time, volumes; IOCTLs on Windows, sysfs on Linux) and network adapters (rx/tx, link speed, address; shown when up and physical or carrying traffic, `a` shows all); DISK/NET in summary strip
- [x] **Keys** Vim-style keymap per Key rules (replaces type-to-search request): single keymap table drives footer, `?` help overlay and consistency tests; rebound k→d, g→z, i→c, a→.
- [x] **Explain** `?` opens "About this view": context-aware explanations (process columns; CPU, memory, disk, network metrics, only those shown on this platform) above the key list; `j`/`k` scroll, `Esc` closes
- [x] **2c** GPU page: every GPU on Windows (DXGI + Task Manager's PDH counters: per-engine usage, dedicated/shared memory; D3DKMT temperature/fan/power for any vendor), NVIDIA deep stats via pure-Go NVML (temp, power vs limit, clocks, P-state, throttle reasons) on Windows + Linux, Linux AMD via amdgpu sysfs; GPU + VRAM in top line, which now drops low-priority items when narrow; stat lines wrap
- [x] **Refactor** to Code standards (behaviour unchanged, proven by golden screen tests + colour comparison): formatting helpers in `view`, shared grid/stat layout; irix→per-core (`--irix` kept as deprecated alias), ring→history, cpu→cpuInfo; all colours behind a `Theme` of named roles; Performance devices behind a `devicePage` interface (one `page_*.go` per device); Processes tab extracted to `processTable`; sampler split into per-area collectors; metrics files and tests grouped per concern
- [x] **2d** Per-process GPU% (busiest engine) and VRAM: Processes columns (only on machines with a GPU; sortable; summed in groups; explained in `?`) and in `syscope ps` (`--sort gpu|vram`, JSON `gpu_percent`/`vram_bytes`). Sources: Windows per-process GPU counters (any vendor), NVML process lists (NVIDIA), DRM fdinfo (Linux AMD/Intel). Idle processes show 0 when the source covers every process, `--` only when genuinely unknown. Processes columns are now a table (`processColumns`), and collectors can enrich processes (`procEnricher`)
- [x] **Themes, part 1** — 10 built-in presets (default, dracula, catppuccin-mocha, tokyo-night, nord, gruvbox-dark, solarized-dark, github-light, high-contrast, colorblind / Okabe–Ito), `--theme`, `syscope themes` preview, `T` live-preview picker popover
- [x] **Wrap-around in menus** — Settings, theme list and Performance devices wrap; processes and help don't (one shared `menuMove` helper; rule in Key rules)
- [x] **Per-volume disks** — volumes of a multi-volume disk appear indented under it in the Performance sidebar with their own read/write graphs and usage (Windows per-volume IOCTL counters, Linux partition counters; macOS and virtual drives show usage only, and say so). Active and response time stay on the physical disk's page. Nested sidebar entries via an optional `nested` interface. Virtual drives (Windows drive letters that aren't `\Device\HarddiskVolume…` partitions, e.g. cloud-sync drives) are kept off physical disks and named in a note on the disk page

### Phase 3 — Developers / power users
- [x] **Details pane** — `Enter` opens a bottom split following the cursor: path, command line, parent/children, user, start time, threads, handles, live I/O rate, CPU/memory/GPU (which GPU and engine)/VRAM; group summary for grouped rows; `Esc` closes before clearing search
- [x] **"Why" hints, part 1** — `internal/insight` rule table shown in the details pane (ℹ explains, ⚠ act): idle cached VRAM, LLM fell back to CPU, big RAM share, video encode/decode, browser-engine child roles, many processes, svchost service names, protected processes (accurate for isolated ones like Secure System), notes on well-known system processes. Checked against real processes on Windows
- [x] **Preferences** — auto-saved TOML in the OS config dir (`syscope config path`): theme, refresh, last tab, sort/direction, grouping, per-core CPU%, details pane, show-hidden devices. Only changed settings are saved (one-run flag overrides aren't); background atomic saves plus a final save on quit; a broken file starts on defaults without overwriting it; `syscope config show|path|reset`, `--config <file>`. `syscope ps` deliberately ignores preferences
- [x] **Process tree + expandable groups** — layouts flat / grouped (`z`) / tree (`t`); `l`/`h` (or →/←) expand/collapse or go to parent, `L`/`H` all; groups start closed, the tree open; search keeps matches' ancestors; processes in parent cycles are never hidden; group rows read `chrome (31)`; layout saved as a preference
- [x] **Settings screen + what persists** — `,` opens Settings (Theme, Refresh, CPU%, Default layout, Default sort, Restore last session), saved as you change them; `T` removed. Only settings persist; dashboard keys are session-only unless "Restore last session" is on, which saves and restores tab, sort, layout, CPU% mode, details pane and hidden devices in a `[session]` section. Footer always keeps `? help`
- [x] **Ports view** — tab `3`: listening sockets (TCP LISTEN and bound UDP) with port, protocol, address and owning process; `.` adds every connection with remote end and state; `/` searches port, name, PID or address; `s`/`S` sort; `d`/`D` end the owner (with y/N); `Enter` jumps to the owner on Processes with its details open (clearing the search or opening its group/tree ancestors when they hide it); `?` explains 0.0.0.0 vs 127.0.0.1 and unknown owners (Linux needs root for other users' sockets). Sockets via gopsutil on every OS (~2 ms per sample). The Processes table's scrolling, columns, search box and kill prompt became shared pieces (`listnav.go`, `kill.go`)
- [x] **Kill tree** — when the process (or any process in a group, or a port's owner) started others, the `d`/`D` prompt offers `t`: end it and every process it started, parents first so they can't respawn children; the count is in the prompt. `y` still ends just the one, so existing muscle memory stays safe (ending `explorer.exe` with `y` doesn't take every desktop app with it). Children must have started after their parent, so reused PIDs aren't mistaken for children
- [x] **Exited (zombie) processes** — Linux/macOS processes that have exited but whose parent hasn't collected them show as `name (exited)` (also in `syscope ps`, and `"exited": true` in JSON); the details pane explains zombies; `d`/`D` on one says it can't be ended instead of appearing to do nothing, and kill tree skips them. Found while testing kill tree in Docker, where the container's PID 1 doesn't reap (use `docker run --init`)
- [x] **Suspend / resume** — `p` asks "Suspend x? [y/N]" (a wrong pick can freeze the desktop), and resumes a suspended process without asking. Refuses syscope itself and its ancestors (shell, terminal), which nothing could then resume. Suspended processes show as `(suspended)` (also in `syscope ps` and JSON) with a details hint; detection matches Task Manager on Windows (every thread waiting with reason Suspended, via NtQuerySystemInformation — so Store apps Windows paused show too) and uses the stopped state on Unix. Quitting with processes this session suspended asks "Resume … before quitting? [Y/n]" (a second Ctrl+C quits without resuming). Kill, suspend and quit share one prompt/job runner
- [x] **Rename + repo** (2026-10-15) — taskman → **syscope** (module, command, text, config folder; old preferences copied); private GitHub repo github.com/benfo/syscope with only `master` pushed, after scanning the full history for personal data

### Phase 1 — POC
- [x] Metrics sampler (gopsutil), CPU deltas, PID-reuse safe cache, Toolhelp names on Windows
- [x] View helpers: sort, filter, group-by-name, formatting
- [x] Graceful / force kill per OS (WM_CLOSE on Windows, SIGTERM/SIGKILL on Unix)
- [x] `syscope ps` (table / JSON) and `syscope kill` (PID / `--name` / `--all`)
- [x] Interactive dashboard: CPU/MEM header, sortable/filterable/groupable table, kill with confirm
- [x] Verified on Windows and Linux (Docker); macOS compiles
