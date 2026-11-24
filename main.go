package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type item struct {
	name, path string
	size       int64
	dir        bool
	errors     int
}
type scanResult struct {
	path      string
	items     []item
	errors    int
	cancelled bool
}
type trashTarget struct {
	path     string
	identity os.FileInfo
}
type trashResult struct {
	path string
	err  error
}

type scanState struct {
	result chan scanResult
	cancel context.CancelFunc
}
type model struct {
	path                            string
	items                           []item
	selected, width, height, errors int
	scan                            *scanState
	status                          string
	showList                        bool
	cache                           map[string]scanResult
	confirm                         *trashTarget
	trashing                        bool
}

func start(path string) (*scanState, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &scanState{result: make(chan scanResult, 1), cancel: cancel}
	go func() { s.result <- scan(ctx, path) }()
	return s, wait(s)
}
func wait(s *scanState) tea.Cmd { return func() tea.Msg { return <-s.result } }

func (m *model) visit(path string) tea.Cmd {
	m.path, m.items, m.selected = path, nil, 0
	if result, ok := m.cache[path]; ok {
		m.items, m.errors = result.items, result.errors
		m.status = "Cached scan · press r to refresh"
		return nil
	}
	m.status = "Scanning… Esc to cancel"
	var cmd tea.Cmd
	m.scan, cmd = start(path)
	return cmd
}

// Finder puts the folder in Trash, not permanent deletion. Never interpolate
// paths into AppleScript; they are passed as argv and rechecked immediately.
func moveToTrash(target trashTarget) tea.Cmd {
	return func() tea.Msg {
		info, err := os.Lstat(target.path)
		if err == nil && (!info.IsDir() || !os.SameFile(target.identity, info)) {
			err = fmt.Errorf("folder changed since confirmation; rescan and try again")
		}
		if err == nil {
			cmd := exec.Command("/usr/bin/osascript", "-e", "on run argv", "-e", "tell application \"Finder\" to delete (POSIX file (item 1 of argv))", "-e", "end run", target.path)
			var out []byte
			out, err = cmd.CombinedOutput()
			if err != nil && len(out) > 0 {
				err = fmt.Errorf("%s", strings.TrimSpace(string(out)))
			}
		}
		return trashResult{path: target.path, err: err}
	}
}
func (m model) Init() tea.Cmd {
	if m.scan != nil {
		return wait(m.scan)
	}
	return nil
}
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
	case scanResult:
		if m.scan != nil && v.path == m.path {
			m.scan = nil
			m.items = v.items
			m.errors = v.errors
			m.selected = 0
			if v.cancelled {
				m.status = "Scan cancelled; results incomplete"
			} else if v.errors > 0 {
				m.status = fmt.Sprintf("%d entries inaccessible; sizes incomplete", v.errors)
			} else {
				m.status = "Scan complete"
				if m.cache == nil {
					m.cache = make(map[string]scanResult)
				}
				m.cache[v.path] = v
			}
		}
	case trashResult:
		m.trashing = false
		if v.err != nil {
			m.status = "Trash failed: " + safe(v.err.Error())
			break
		}
		// A folder removal changes its ancestors and may affect hard-link accounting.
		m.cache = make(map[string]scanResult)
		m.status = "Moved to Trash; rescanning…"
		var cmd tea.Cmd
		m.scan, cmd = start(m.path)
		return m, cmd
	case tea.KeyMsg:
		if m.confirm != nil {
			switch v.String() {
			case "y":
				target := *m.confirm
				m.confirm = nil
				m.trashing = true
				m.status = "Moving folder to Trash…"
				return m, moveToTrash(target)
			case "n", "esc":
				m.confirm = nil
				m.status = "Trash cancelled"
			}
			return m, nil
		}
		if m.trashing {
			return m, nil
		}
		switch v.String() {
		case "q", "ctrl+c":
			if m.scan != nil {
				m.scan.cancel()
			}
			return m, tea.Quit
		case "esc":
			if m.scan != nil {
				m.scan.cancel()
			}
		case "j", "down":
			if m.selected+1 < len(m.items) {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		case "g", "home":
			m.selected = 0
		case "G", "end":
			if len(m.items) > 0 {
				m.selected = len(m.items) - 1
			}
		case "tab":
			m.showList = !m.showList
		case "d":
			if m.scan == nil && len(m.items) > 0 {
				it := m.items[m.selected]
				if !it.dir || filepath.Dir(it.path) != m.path {
					m.status = "Select a direct child folder to move to Trash"
					break
				}
				info, err := os.Lstat(it.path)
				if err != nil || !info.IsDir() {
					m.status = "Folder unavailable or changed; press r to rescan"
					break
				}
				m.confirm = &trashTarget{path: it.path, identity: info}
			}
		case "r":
			if m.scan == nil {
				// A refresh can change hard-link attribution in related folders.
				m.cache = make(map[string]scanResult)
				m.status = "Scanning… Esc to cancel"
				var cmd tea.Cmd
				m.scan, cmd = start(m.path)
				return m, cmd
			}
		case "enter", "right":
			if m.scan == nil && len(m.items) > 0 && m.items[m.selected].dir {
				cmd := m.visit(m.items[m.selected].path)
				return m, cmd
			}
		case "backspace", "left":
			if m.scan == nil {
				parent := filepath.Dir(m.path)
				if parent != m.path {
					cmd := m.visit(parent)
					return m, cmd
				}
			}
		}
	}
	return m, nil
}
func human(n int64) string {
	v := float64(n)
	u := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	i := 0
	for v >= 1024 && i < len(u)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", v, u[i])
}

var palette = []string{"#573B98", "#176A82", "#28745E", "#94485F", "#92602D", "#445A9B", "#784282", "#2C7075", "#845037", "#3E718F", "#68569B", "#667A33", "#8A4356", "#356A62", "#805C31", "#594C85"}

type cell struct {
	bg       int
	text     string
	selected bool
}

func drawMap(items []item, sel, w, h int) string {
	if w < 1 || h < 1 {
		return ""
	}
	grid := make([][]cell, h)
	for y := range grid {
		grid[y] = make([]cell, w)
		for x := range grid[y] {
			grid[y][x].bg = -1
		}
	}
	var fill func(int, int, int, int, int, int)
	fill = func(a, b, x, y, ww, hh int) {
		if a >= b || ww < 1 || hh < 1 {
			return
		}
		if b-a == 1 {
			// A one-cell gutter creates clear compartments on terminals that support color.
			bw, bh := ww, hh
			if bw > 5 {
				bw--
			}
			if bh > 3 {
				bh--
			}
			for yy := y; yy < y+bh; yy++ {
				for xx := x; xx < x+bw; xx++ {
					c := cell{bg: a % len(palette), selected: a == sel}
					// Fill the tile with its motif; borders and labels are overlaid below.
					if yy > y && yy < y+bh-1 && xx > x && xx < x+bw-1 {
						switch a % 8 {
						case 0:
							if (xx-x)/2%2 == (yy-y)%2 {
								c.text = "░"
							}
						case 1:
							if (xx-x+2*(yy-y))%8 == 0 {
								c.text = "·"
							}
						case 2:
							if (xx-x+yy-y)%7 == 0 {
								c.text = "╱"
							}
						case 3:
							if (xx-x)%6 == 0 && (yy-y)%2 == 0 {
								c.text = "┼"
							}
						case 4:
							if (xx-x+yy-y)%6 == 0 {
								c.text = "╲"
							}
						case 5:
							if (yy-y)%3 == 0 && (xx-x)%4 < 2 {
								c.text = "─"
							}
						case 6:
							if (xx-x)%5 == 0 && (yy-y)%3 == 0 {
								c.text = "◇"
							}
						case 7:
							if (xx-x+2*(yy-y))%6 == 0 {
								c.text = "✦"
							}
						}
					}
					grid[yy][xx] = c
				}
			}
			if bw >= 4 && bh >= 3 {
				for xx := x + 1; xx < x+bw-1; xx++ {
					grid[y][xx].text = "─"
					grid[y+bh-1][xx].text = "─"
				}
				for yy := y + 1; yy < y+bh-1; yy++ {
					grid[yy][x].text = "│"
					grid[yy][x+bw-1].text = "│"
				}
				grid[y][x].text, grid[y][x+bw-1].text = "┌", "┐"
				grid[y+bh-1][x].text, grid[y+bh-1][x+bw-1].text = "└", "┘"
				if a == sel {
					for xx := x + 1; xx < x+bw-1; xx++ {
						grid[y][xx].text = "━"
						grid[y+bh-1][xx].text = "━"
					}
					for yy := y + 1; yy < y+bh-1; yy++ {
						grid[yy][x].text = "┃"
						grid[yy][x+bw-1].text = "┃"
					}
					grid[y][x].text, grid[y][x+bw-1].text = "┏", "┓"
					grid[y+bh-1][x].text, grid[y+bh-1][x+bw-1].text = "┗", "┛"
				}
				name := safe(items[a].name)
				if items[a].dir {
					name += "/"
				}
				writeCells(grid, y+1, x+1, bw-2, runewidth.Truncate(name, bw-2, "…"))
				if bh >= 4 {
					writeCells(grid, y+2, x+1, bw-2, " "+human(items[a].size)+" ")
				}
			} else if bw > 4 {
				writeCells(grid, y, x+1, bw-2, runewidth.Truncate(safe(items[a].name), bw-2, "…"))
			}
			return
		}
		var total, part float64
		for i := a; i < b; i++ {
			v := float64(items[i].size)
			if v < 1 {
				v = 1
			}
			total += v
		}
		k := a
		for k < b-1 && (k == a || part < total/2) {
			v := float64(items[k].size)
			if v < 1 {
				v = 1
			}
			part += v
			k++
		}
		if ww >= hh*2 && ww >= 2 {
			n := int(float64(ww) * part / total)
			if n < 1 {
				n = 1
			}
			if n >= ww {
				n = ww - 1
			}
			fill(a, k, x, y, n, hh)
			fill(k, b, x+n, y, ww-n, hh)
		} else if hh >= 2 {
			n := int(float64(hh) * part / total)
			if n < 1 {
				n = 1
			}
			if n >= hh {
				n = hh - 1
			}
			fill(a, k, x, y, ww, n)
			fill(k, b, x, y+n, ww, hh-n)
		} else if ww >= 2 {
			n := int(float64(ww) * part / total)
			if n < 1 {
				n = 1
			}
			if n >= ww {
				n = ww - 1
			}
			fill(a, k, x, y, n, hh)
			fill(k, b, x+n, y, ww-n, hh)
		}
	}
	fill(0, len(items), 0, 0, w, h)
	var out strings.Builder
	for y, row := range grid {
		for x := 0; x < len(row); {
			c := row[x]
			end := x + 1
			for end < len(row) && row[end].bg == c.bg && row[end].selected == c.selected {
				end++
			}
			var s strings.Builder
			for _, v := range row[x:end] {
				if v.text == "" {
					s.WriteByte(' ')
				} else {
					s.WriteString(v.text)
				}
			}
			style := lipgloss.NewStyle()
			if c.bg >= 0 {
				style = style.Background(lipgloss.Color(palette[c.bg])).Foreground(lipgloss.Color("#FFFFFF"))
				if c.selected {
					style = style.Bold(true)
				}
			}
			out.WriteString(style.Render(s.String()))
			x = end
		}
		if y < h-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '�'
		}
		return r
	}, s)
}
func writeCells(g [][]cell, y, x, width int, s string) {
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if rw != 1 {
			r = '?'
		}
		if width <= 0 {
			return
		}
		g[y][x].text = string(r)
		x++
		width--
	}
}
func clip(s string, width int) string {
	if width < 1 {
		return ""
	}
	return runewidth.Truncate(s, width, "…")
}
func (m model) View() string {
	if m.width < 25 || m.height < 9 {
		return "Resize terminal (minimum 25 × 9). q to quit"
	}
	w := m.width - 1
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#B8A2FF"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#A3B1C9"))
	title := accent.Render("◆ DISK ATLAS") + muted.Render("   /   storage explorer")
	var stat syscall.Statfs_t
	disk := "capacity unavailable"
	if syscall.Statfs(m.path, &stat) == nil {
		disk = fmt.Sprintf("%s free", human(int64(stat.Bavail)*int64(stat.Bsize)))
	}
	var total int64
	for _, it := range m.items {
		total += it.size
	}
	meta := fmt.Sprintf(" %s   ·   %s scanned   ·   %d items   ·   %s", disk, human(total), len(m.items), m.status)
	path := "  " + safe(m.path)
	header := title + "\n" + muted.Render(clip(path, w)) + "\n" + muted.Render(clip(meta, w))
	detail := "  Select an item to inspect it"
	if m.scan != nil && len(m.items) == 0 {
		detail = "  Scanning files… results will appear when complete"
	}
	if len(m.items) > 0 {
		it := m.items[m.selected]
		kind := "file"
		if it.dir {
			kind = "folder"
		}
		pct := float64(0)
		if total > 0 {
			pct = 100 * float64(it.size) / float64(total)
		}
		detail = fmt.Sprintf("  SELECTED   %s   ·   %s   ·   %.1f%%   ·   %s   [%d/%d]", safe(it.name), human(it.size), pct, kind, m.selected+1, len(m.items))
		if it.errors > 0 {
			detail += "   ! incomplete"
		}
	}
	height := m.height - 7
	body := ""
	if m.showList {
		lines := make([]string, height)
		start := 0
		if m.selected >= height {
			start = m.selected - height + 1
		}
		for row := 0; row < height; row++ {
			i := start + row
			if i >= len(m.items) {
				break
			}
			it := m.items[i]
			marker := "  "
			if i == m.selected {
				marker = "❯ "
			}
			suffix := ""
			if it.dir {
				suffix = "/"
			}
			line := fmt.Sprintf(" %s%-12s %s%s", marker, human(it.size), safe(it.name), suffix)
			lines[row] = clip(line, w)
		}
		body = strings.Join(lines, "\n")
	} else {
		body = drawMap(m.items, m.selected, w, height)
	}
	footer := clip(" ↑↓ select   ↵ open   ← parent   tab view   d trash   r refresh   q quit", w)
	if m.confirm != nil {
		detail = "  MOVE TO TRASH? " + safe(m.confirm.path)
		footer = clip(" This folder and its contents will move to Trash.  y confirm   n / Esc cancel", w)
	}
	return header + "\n" + accent.Render(clip(detail, w)) + "\n" + muted.Render(" "+strings.Repeat("─", w-1)) + "\n" + body + "\n" + muted.Render(footer)
}
func main() {
	path := ""
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: diskatlas [directory]")
		os.Exit(2)
	}
	if len(os.Args) == 2 {
		path = os.Args[1]
	} else {
		path, _ = os.UserHomeDir()
	}
	var err error
	path, err = filepath.Abs(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		fmt.Fprintln(os.Stderr, "not a directory:", path)
		os.Exit(1)
	}
	s, _ := start(path)
	m := model{path: path, scan: s, status: "Scanning… Esc to cancel"}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err = p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
