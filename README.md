<p align="center">
  <img src="assets/logo.png" alt="sono - Node.js &amp; package manager toolkit" width="440">
</p>

<p align="center">Node.js &amp; package manager toolkit</p>

sono is a single, self-contained Go binary that manages Node.js versions and package managers from the terminal.
It plays the same role as nvm, fnm or volta: run it with no arguments for a full-screen interactive TUI, or pass a subcommand (`sono install 20`, `sono use 20`, `sono ls`) for scripting.
No Node.js needs to be installed on the machine beforehand.

## Features

- Browse, install, activate and uninstall Node.js versions straight from nodejs.org.
- Filter by LTS / non-LTS / installed, search by version prefix, scrollable list.
- End-of-support dates per release, and an "update available" marker when a newer patch exists in the same minor line.
- SHA256-verified downloads (a tampered checksum fails cleanly, with nothing extracted).
- Instant activation through an atomic `current` symlink (reversible, no root).
- Package managers: install, activate and uninstall pnpm and yarn (SHA512-verified npm packages), run through shims that use the active Node.
- Tarball cache footer with a manual purge and an age-based auto-purge, both configurable from the TUI.
- Terminal-native touches: live download progress, inline confirm prompts for destructive actions, a success/error status line, and a PATH helper that copies the `export` line (OSC 52) and only shows while the PATH is not set up.

## Interface

sono is a keyboard-driven TUI with two sections (Node and Package managers). Key bindings:

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | move selection |
| `/` | search by version prefix |
| `f` | cycle filter (Node) / switch package manager (PM) |
| `v` | toggle compact / all versions (Node) |
| `enter` or `i` | install (or update) the selected version |
| `a` | activate the selected version |
| `u` | uninstall (asks for confirmation) |
| `c` | clear the tarball cache (Node) |
| `p` | toggle cache auto-purge (Node) |
| `y` | copy the PATH `export` line |
| `tab` | switch section |
| `?` | toggle help |
| `q` | quit |

## Command line

The same actions are available as non-interactive subcommands, for scripting and quick one-offs. Results print to stdout; download progress goes to stderr, so piping stays clean.

```sh
sono install 20            # install latest v20.x (also: 20.11, v20.11.0, lts, latest)
sono install lts --use     # install and activate in one step
sono use 20                # activate an installed version
sono uninstall 20          # remove an installed version
sono ls                    # installed versions (* = active)
sono ls-remote --lts       # available versions (flags: --lts, --nonlts, --all)
sono current               # print the active version
sono path                  # print the PATH export line

sono pm ls-remote pnpm 9   # available pnpm 9.x
sono pm install pnpm 9 --use
sono pm use yarn 4

sono cache info            # cached tarball count and size
sono cache purge           # delete cached tarballs

sono help                  # full command reference
```

Version arguments are resolved to the highest match, so `sono install 20` picks the latest v20 release, and `sono use 20` activates the newest installed v20.

## Requirements

- Go 1.26 or newer to build.
- Third-party dependencies (all pure Go, so the build never needs Node): `github.com/ulikunitz/xz` to decompress Node's `.tar.xz` archives, and `github.com/charmbracelet/bubbletea` (+ `bubbles`, `lipgloss`) for the terminal UI.

## Build and run

```sh
go build -o sono .
./sono   # launches the interactive TUI
```

sono runs entirely in the terminal — no server, no browser, no address to configure.

## PATH setup

Add this line to your `~/.bashrc` (or shell equivalent), then open a new terminal:

```sh
export PATH="$HOME/.sono/current/bin:$HOME/.sono/shims:$PATH"
```

- `~/.sono/current/bin` exposes the active Node.js (`node`, `npm`, `npx`).
- `~/.sono/shims` exposes the active package managers (`pnpm`, `yarn`, ...).

The TUI shows this exact line (press `y` to copy it), and only reminds you while it is missing.

## Data layout

Everything lives under `~/.sono/`:

```
~/.sono/
  versions/        installed Node.js versions (extracted tarballs)
  current -> ...    symlink to the active Node.js version
  shims/           active package-manager commands (on PATH)
  pm/              installed pnpm / yarn versions + registry caches
  cache/           downloaded Node.js tarballs (reusable, purgeable)
  index.json       cached nodejs.org version index
  schedule.json    cached Node.js release schedule (end-of-support dates)
  config.json      persisted settings (cache auto-purge)
```

## How it works

- Node.js versions are downloaded as the official `.tar.xz`, verified against `SHASUMS256.txt`, and extracted into `~/.sono/versions/`.
The active version is just the `current` symlink, so switching is instant and reversible.
- Package managers are downloaded as their npm package `.tgz`, verified against the registry SHA512 integrity, and extracted into `~/.sono/pm/`.
Activating one writes shims into `~/.sono/shims/` that run the package manager with the active Node.

## Tech

- Core: Go standard library, plus `github.com/ulikunitz/xz` for `.tar.xz` decompression.
- UI: [Bubble Tea](https://github.com/charmbracelet/bubbletea) (+ bubbles, lipgloss) — the Elm-style TUI framework. All pure Go, so sono still builds and runs without Node and ships as a single binary.

## Design docs

Deeper design notes live in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/PLAN.md](docs/PLAN.md) (written in French).

## License

Released under the MIT License.
See [LICENSE](LICENSE).
