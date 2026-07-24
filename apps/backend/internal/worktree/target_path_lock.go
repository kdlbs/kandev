package worktree

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const windowsGOOS = "windows"

type targetPathLockEntry struct {
	token chan struct{}
	refs  int
}

type targetPathLockRegistry struct {
	mu      sync.Mutex
	entries map[string]*targetPathLockEntry
}

var worktreeTargetPathLocks = targetPathLockRegistry{
	entries: make(map[string]*targetPathLockEntry),
}

func acquireWorktreeTargetPath(ctx context.Context, path string) (func(), error) {
	key, err := normalizedWorktreeTargetPath(path)
	if err != nil {
		return nil, err
	}
	entry := worktreeTargetPathLocks.reference(key)
	select {
	case <-entry.token:
		var once sync.Once
		return func() {
			once.Do(func() {
				entry.token <- struct{}{}
				worktreeTargetPathLocks.unreference(key, entry)
			})
		}, nil
	case <-ctx.Done():
		worktreeTargetPathLocks.unreference(key, entry)
		return nil, ctx.Err()
	}
}

func normalizedWorktreeTargetPath(path string) (string, error) {
	return normalizedWorktreeTargetPathForOS(path, runtime.GOOS)
}

func normalizedWorktreeTargetPathForOS(path, goos string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve worktree target path: %w", err)
	}
	key := filepath.Clean(absPath)
	if goos == windowsGOOS {
		key = strings.ToLower(key)
	}
	return key, nil
}

func (r *targetPathLockRegistry) reference(key string) *targetPathLockEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entries[key]
	if entry == nil {
		entry = &targetPathLockEntry{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		r.entries[key] = entry
	}
	entry.refs++
	return entry
}

func (r *targetPathLockRegistry) unreference(key string, entry *targetPathLockEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && r.entries[key] == entry {
		delete(r.entries, key)
	}
}
