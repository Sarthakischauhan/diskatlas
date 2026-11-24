# Disk Atlas

**Explore disk usage as a colorful, patterned treemap—without leaving your terminal.**

Disk Atlas is a macOS disk-usage browser built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). Scan a directory, see which entries occupy the most allocated space, and drill down to investigate.

![Disk Atlas treemap showing patterned, size-proportional tiles](docs/images/treemap.png)

*The screenshots show actual terminal output captured with a headless terminal and rendered as images. The example files are synthetic.*

## Highlights

- **Treemap and list views** — compare direct children by allocated size; switch views without rescanning.
- **Drill down** — open a folder to scan its contents, or return to its parent.
- **Visual selection** — color and patterns distinguish tiles; gold highlighting, a heavy border, and a `▶` marker identify the selected item.
- **Faster scans and cached navigation** — up to four top-level entries are scanned concurrently, and completed scans are reused during the current session; press `r` for fresh results.
- **Trash with confirmation** — move a selected folder to macOS Trash after explicitly confirming. Nothing is removed automatically.
- **Visible limitations** — inaccessible entries and cancelled scans are reported as incomplete.

## Requirements

- macOS and a terminal with Unicode support. A color-capable terminal is recommended.
- Go **1.25.4 or newer** to build from source. Go downloads the dependencies listed in `go.mod` on the first build.
- `make` is optional; the equivalent Go commands are shown below.

## Install

```sh
git clone https://github.com/Sarthakischauhan/diskatlas.git
cd diskatlas
make
./diskatlas
```

To make the command available from anywhere, install a copy to a directory on your `PATH`:

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 diskatlas "$HOME/.local/bin/diskatlas"
```

If `~/.local/bin` is not on your `PATH`, add it in your shell configuration. Alternatively, without `make`, run `go build -o diskatlas .`.

## Usage

New to Disk Atlas? Follow the [step-by-step user guide](docs/user-guide.md) for a walkthrough and troubleshooting.

```sh
diskatlas                       # scan your home directory
diskatlas ~/Downloads           # scan a specific directory
diskatlas /System/Volumes/Data  # scan the macOS data volume
```

The map appears when the scan completes. Large directories can take time. Each tile represents one **direct child**; its area is proportional to the allocated space found beneath it. Open a directory to scan the next level. Very small entries may not get a visible tile; use the list view to select them.

![Disk Atlas list view showing entries sorted by size](docs/images/list.png)

### Controls

| Key | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Select an entry |
| `Enter` / `→` | Open the selected directory |
| `Backspace` / `←` | Scan the parent directory |
| `Tab` | Toggle treemap and list |
| `g` / `Home`, `G` / `End` | Select the first or last entry |
| `r` | Discard the current cached scan and rescan |
| `d` | Ask to move the selected folder to Trash; confirm with `y` or cancel with `n` / `Esc` |
| `Esc` | Cancel the current scan and show partial results |
| `q` / `Ctrl-C` | Quit |

### Cache and Trash

Successful scans are cached **in memory for this app session only**. Revisiting a directory reuses its cached result, which can be stale if files changed outside Disk Atlas. Press `r` to discard all session caches and force a new scan. Cancelled or incomplete scans are not cached. After a successful Trash operation, all session caches are discarded and the current directory is rescanned. Independent direct children are scanned concurrently (up to four workers); a directory dominated by one child may see little speedup.

Select a folder and press `d`; check the displayed path and press `y` to move it and its contents to the macOS Trash, or `n` / `Esc` to cancel. Disk Atlas asks Finder to perform the move, so macOS may ask for Automation permission. The app does not permanently delete items; restore them using Finder's Trash. Finder can refuse a move because of permissions, volume limitations, or other errors. Files and symlinks cannot be trashed using this shortcut.

## Understanding the numbers

Disk Atlas sums allocated filesystem blocks, not the logical length of files. It stays on the starting filesystem, does not follow symbolic links during traversal, and counts hard-linked inodes once per scan. The displayed free-space value comes from the filesystem rather than the scanned entries.

On macOS, privacy permissions can prevent scanning some directories. You can grant your terminal **Full Disk Access** in System Settings → Privacy & Security, if appropriate, then rescan. An incomplete scan is marked in the status or selected entry.

APFS clones, snapshots, other volumes, shared blocks, and filesystem metadata are not fully accounted for. As a result, totals can differ from Finder, `df`, or the space actually recovered by deleting a file. A whole-volume scan can take a long time and is not guaranteed to reveal every byte.

## Terminal appearance

Disk Atlas uses terminal colors and Unicode patterns. Actual colors depend on your terminal, theme, and color support. Glass-like transparency and blur are controlled by the terminal application; the colored tile backgrounds themselves are opaque.

## Development

```sh
make test        # go test ./... and go vet ./...
make             # build ./diskatlas
go test ./...    # unit tests only
```

The scanner is in `scan.go`, the terminal interface is in `main.go`, and tests are in `main_test.go`. Disk Atlas is an early-stage project; it has not been comprehensively tested across macOS versions, terminal applications, or very large filesystems.
