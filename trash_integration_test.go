package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Opt in because Finder Automation access may require an interactive prompt.
// The disposable test folder is moved to the user's Trash.
func TestFinderTrashIntegration(t *testing.T) {
	if os.Getenv("DISKATLAS_TEST_FINDER") != "1" {
		t.Skip("set DISKATLAS_TEST_FINDER=1 to test Finder automation")
	}
	root := t.TempDir()
	path := filepath.Join(root, "Disk Atlas test ' quotes")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	result := moveToTrash(trashTarget{path: path, identity: info})().(trashResult)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("folder remains at original path: %v", err)
	}
}
