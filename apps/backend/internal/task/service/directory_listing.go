package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirectoryEntry is one immediate child of a listed directory.
type DirectoryEntry struct {
	Name string
	Path string // absolute path
}

// DirectoryListing is the result of ListDirectory: the absolute path that was
// listed, the parent (empty when at root), and the immediate subdirectory
// children sorted alphabetically.
type DirectoryListing struct {
	Path    string
	Parent  string
	Entries []DirectoryEntry
}

// ListDirectory returns the immediate subdirectories of path. When path is
// empty it falls back to $HOME. Hidden directories (starting with ".") are
// excluded. Used by the folder picker for repo-less tasks.
func (s *Service) ListDirectory(ctx context.Context, path string) (DirectoryListing, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return DirectoryListing{}, fmt.Errorf("resolve home: %w", err)
		}
		path = home
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("invalid path: %w", err)
	}
	abs = filepath.Clean(abs)

	info, err := os.Stat(abs)
	if err != nil {
		return DirectoryListing{}, err
	}
	if !info.IsDir() {
		return DirectoryListing{}, fmt.Errorf("not a directory: %s", abs)
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return DirectoryListing{}, err
	}

	out := make([]DirectoryEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, DirectoryEntry{
			Name: name,
			Path: filepath.Join(abs, name),
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })

	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}

	_ = ctx
	return DirectoryListing{
		Path:    abs,
		Parent:  parent,
		Entries: out,
	}, nil
}
