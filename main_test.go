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
func TestSafeControlCharacters(t *testing.T) {
	if strings.ContainsRune(safe("a\x1bb"), '\x1b') {
		t.Fatal("terminal escape not sanitized")
	}
}
