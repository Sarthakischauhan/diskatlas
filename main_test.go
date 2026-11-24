package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestScanAndSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "folder", "file"), make([]byte, 8192), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "folder"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	r := scan(context.Background(), root)
	if r.errors != 0 || len(r.items) != 2 {
		t.Fatalf("scan: %+v", r)
	}
	for _, it := range r.items {
		if it.name == "link" && it.dir {
			t.Fatal("followed symlink")
		}
		if it.name == "folder" && it.size <= 0 {
			t.Fatal("empty allocated size")
		}
	}
}
func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := scan(ctx, t.TempDir())
	if !r.cancelled {
		t.Fatal("cancelled scan not reported")
	}
}
func TestMapAndSelection(t *testing.T) {
	items := []item{{name: "alpha", size: 100, dir: true}, {name: "beta", size: 20}}
	s := drawMap(items, 0, 40, 10)
	if !strings.Contains(s, "alpha") || !strings.Contains(s, "beta") {
		t.Fatal("missing labels")
	}
	m := model{items: items, path: "/tmp", width: 80, height: 24}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if next.(model).selected != 1 {
		t.Fatal("selection did not advance")
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !next.(model).showList {
		t.Fatal("list did not toggle")
	}
}
func TestCacheNavigationAndRefresh(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	m := model{path: child, cache: map[string]scanResult{root: {path: root, items: []item{{name: "cached", size: 42}}}}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	got := next.(model)
	if cmd != nil || got.scan != nil || len(got.items) != 1 || !strings.Contains(got.status, "Cached") {
		t.Fatalf("cache not used: %+v", got)
	}
	next, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got = next.(model)
	if cmd == nil || got.scan == nil {
		t.Fatal("refresh must start new scan")
	}
	got.scan.cancel()
	cmd()
}

func TestTrashConfirmationAndChangedFolder(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "folder")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	m := model{path: root, items: []item{{name: "folder", path: folder, dir: true}}, width: 100, height: 24}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	got := next.(model)
	if got.confirm == nil || cmd != nil || !strings.Contains(got.View(), "MOVE TO TRASH") {
		t.Fatal("missing confirmation")
	}
	next, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if next.(model).confirm != nil || cmd != nil {
		t.Fatal("cancel failed")
	}
	old := got.confirm.identity
	if err := os.Rename(folder, folder+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	result := moveToTrash(trashTarget{path: folder, identity: old})().(trashResult)
	if result.err == nil {
		t.Fatal("changed folder must not be trashed")
	}
	if _, err := os.Stat(folder); err != nil {
		t.Fatal(err)
	}
}

func TestSafeControlCharacters(t *testing.T) {
	if strings.ContainsRune(safe("a\x1bb"), '\x1b') {
		t.Fatal("terminal escape not sanitized")
	}
}
