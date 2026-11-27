package agenthistory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openHistoryRoot resolves the configured root once, then pins each real
// directory component. Later operations never reopen its lexical ancestors.
func openHistoryRoot(path string) (*os.Root, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	anchor := filepath.VolumeName(resolved) + string(filepath.Separator)
	root, err := os.OpenRoot(anchor)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(anchor, resolved)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if relative == "." {
		return root, nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		next, err := openRealSubdirectory(root, component)
		_ = root.Close()
		if err != nil || next == nil {
			return nil, err
		}
		root = next
	}
	return root, nil
}

// openRealSubdirectory uses the same Lstat/OpenRoot/SameFile boundary as
// storage/gocache: a swapped or symlinked entry cannot redirect a pinned handle.
func openRealSubdirectory(parent *os.Root, name string) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !before.IsDir() {
		return nil, nil
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		_ = child.Close()
		return nil, errors.Join(err, fmt.Errorf("history directory changed: %s", name))
	}
	return child, nil
}

func historyEntries(root *os.Root) ([]os.DirEntry, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return f.ReadDir(-1)
}

func openRegularFile(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	before, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil || !before.Mode().IsRegular() {
		return nil, nil, err
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		_ = f.Close()
		return nil, nil, errors.Join(err, fmt.Errorf("history file changed: %s", name))
	}
	return f, after, nil
}

func removeObservedEntry(parent *os.Root, name string, expected os.FileInfo) (bool, error) {
	current, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !os.SameFile(expected, current) || current.Mode() != expected.Mode() {
		return false, fmt.Errorf("history entry changed before removal: %s", name)
	}
	if err := parent.Remove(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func removeRegularFile(root *os.Root, name string) (bool, error) {
	f, info, err := openRegularFile(root, name)
	if err != nil || f == nil {
		return false, err
	}
	_ = f.Close()
	return removeObservedEntry(root, name, info)
}

func removePinnedDirectory(ctx context.Context, parent *os.Root, name string, child *os.Root) (bool, error) {
	info, err := child.Stat(".")
	if err != nil {
		return false, err
	}
	entries, err := historyEntries(child)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if err := context.Cause(ctx); err != nil {
			return false, err
		}
		if err := child.RemoveAll(entry.Name()); err != nil {
			return false, err
		}
	}
	return removeObservedEntry(parent, name, info)
}
