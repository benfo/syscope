# syscope

A process manager and performance monitor for the terminal, in the spirit of
Windows Task Manager. One program for Windows, Linux and macOS, aimed at power
users, developers, local-LLM users and gamers.

- **Processes**: live table with CPU, memory and GPU per process, a tree view,
  search, and ending, killing, suspending or resuming processes (always asks first)
- **Performance**: CPU core grid (P/E cores), memory, disks, network and GPU,
  with graphs
- **Ports**: who is listening on what, and which process owns each connection
- Vim-style keys, `?` help that explains every number on screen, colour themes
- `syscope ps` and `syscope kill` for scripts

## Install

**Windows** (PowerShell):

```powershell
irm https://raw.githubusercontent.com/benfo/syscope/master/install.ps1 | iex
```

**macOS and Linux**:

```sh
curl -fsSL https://raw.githubusercontent.com/benfo/syscope/master/install.sh | sh
```

Both download the latest [release](https://github.com/benfo/syscope/releases),
check its SHA-256 checksum and install `syscope` for your user. No admin
rights are needed:

| OS | Installs to |
|---|---|
| Windows | `%LOCALAPPDATA%\Programs\syscope` (added to your user Path) |
| macOS, Linux | `~/.local/bin` (you're told if it isn't on your PATH) |

Set `SYSCOPE_INSTALL_DIR` to choose another folder, or `SYSCOPE_VERSION`
(e.g. `v0.1.0`) to install a specific release. Run the same command again to
upgrade.

<details>
<summary>Other ways to install</summary>

**With Go** (1.27 or newer):

```sh
go install github.com/benfo/syscope/cmd/syscope@latest
```

**By hand**: download the archive for your OS and CPU from
[Releases](https://github.com/benfo/syscope/releases), extract it and put
`syscope` (or `syscope.exe`) somewhere on your PATH. Release builds aren't
signed yet, so Windows SmartScreen and macOS Gatekeeper may warn about a file
downloaded with a browser. The install scripts avoid that.

**Alpine and other musl-based Linux**: the Linux build needs glibc's loader
(it loads the NVIDIA driver library for GPU stats). Run `apk add gcompat` first.

</details>

## Use

```sh
syscope              # live dashboard
syscope ps           # list processes once (see syscope ps --help)
syscope kill <name>  # end processes by name or PID
syscope themes       # preview colour themes
```

In the dashboard, `1`–`3` switch tabs, `j`/`k` move, `/` searches, `s` sorts,
`,` opens Settings and `?` shows help for whatever you're looking at. `q` quits.

## Uninstall

Delete the executable. On Windows, also remove its folder from your user Path.
Preferences live in `syscope config path`; delete that file too if you like.
