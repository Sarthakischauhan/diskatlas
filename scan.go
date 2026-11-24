package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"syscall"
)

type inodeKey struct{ dev, ino uint64 }
type scannedChild struct {
	item      item
	links     map[inodeKey]int64
	cancelled bool
}

// scan partitions work by direct child. Shared hard links are reconciled in
// directory-name order after workers finish, preserving deterministic totals.
func scan(ctx context.Context, root string) scanResult {
	result := scanResult{path: root}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		result.errors++
		return result
	}
	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok {
		result.errors++
		return result
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		result.errors++
		return result
	}
	children := make([]scannedChild, len(entries))
	jobs := make(chan int)
	workers := runtime.GOMAXPROCS(0)
	if workers > 4 {
		workers = 4
	}
	if workers > len(entries) {
		workers = len(entries)
	}
	var wg sync.WaitGroup
	for n := 0; n < workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				e := entries[i]
				path := filepath.Join(root, e.Name())
				c := scannedChild{item: item{name: e.Name(), path: path, dir: e.IsDir()}}
				err := filepath.WalkDir(path, func(path string, d fs.DirEntry, walkErr error) error {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if walkErr != nil {
						c.item.errors++
						return nil
					}
					info, err := d.Info()
					if err != nil {
						c.item.errors++
						return nil
					}
					st, ok := info.Sys().(*syscall.Stat_t)
					if !ok {
						c.item.errors++
						return nil
					}
					if uint64(st.Dev) != uint64(rootStat.Dev) {
						if d.IsDir() {
							return filepath.SkipDir
						}
						return nil
					}
					blocks := int64(st.Blocks) * 512
					// Directories typically have multiple links as part of their normal structure,
					// but cannot be hard-linked by users; deduplicate only non-directories.
					if !info.IsDir() && st.Nlink > 1 {
						if c.links == nil {
							c.links = make(map[inodeKey]int64)
						}
						c.links[inodeKey{uint64(st.Dev), uint64(st.Ino)}] = blocks
					} else {
						c.item.size += blocks
					}
					return nil
				})
				if err != nil {
					if ctx.Err() != nil {
						c.cancelled = true
					} else {
						c.item.errors++
					}
				}
				children[i] = c
			}
		}()
	}
dispatch:
	for i := range entries {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	seen := make(map[inodeKey]struct{})
	for _, c := range children {
		if c.item.path == "" {
			continue
		}
		for key, size := range c.links {
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				c.item.size += size
			}
		}
		result.items = append(result.items, c.item)
		result.errors += c.item.errors
		if c.cancelled {
			result.cancelled = true
		}
	}
	if ctx.Err() != nil {
		result.cancelled = true
	}
	sort.Slice(result.items, func(i, j int) bool {
		if result.items[i].size == result.items[j].size {
			return result.items[i].name < result.items[j].name
		}
		return result.items[i].size > result.items[j].size
	})
	return result
}
