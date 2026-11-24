# Disk Atlas

A read-only macOS terminal disk-usage browser built in Go with Bubble Tea and Lip Gloss. The scanner and treemap are part of this project; it does not run dfdu. The earlier C prototype is in `legacy/`.

```sh
make
./diskatlas                  # home directory
./diskatlas ~/Downloads     # or another directory
make test
```

`~/.local/bin/diskatlas` is a symlink to the compiled binary. Move the project only after updating that symlink.

Arrow keys or j/k select, Enter/right opens a folder, Backspace/left goes up, Tab toggles treemap/list, r rescans, Esc cancels a scan, q quits. Tiles use different colors and patterns; the selected tile has a heavy border. Scan runs in the background, but the map appears only once scanning finishes. This is an early version, not a fully verified production tool.

The terminal supplies its own background: a terminal UI cannot create true translucent glass or blur behind its cells. You can enable background transparency or blur in your terminal app; colored treemap cells remain opaque.

Sizes are allocated filesystem blocks. Scans stay on the starting filesystem, do not follow symlinks, and count hard links once per scan. Permission errors are reported, and partial results are marked incomplete. macOS Full Disk Access may be needed for protected directories. APFS clones, snapshots, system volumes, and shared blocks are not fully accounted for, so sums may differ from Finder and `df`. The scan is read-only; it does not delete anything. Very small entries may not be visible in the treemap; use the list view to select them.
