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
// listed, the parent (empty when at the root of an allowed prefix), and the
// immediate subdirectory children sorted alphabetically.
type DirectoryListing struct {
	Path    string
	Parent  string
	Entries []DirectoryEntry
}

// ListDirectory returns the immediate subdirectories of path. When path is
// empty it falls back to the first configured discovery root (typically
// $HOME). The path must be inside one of the configured discovery roots; any
// path outside (or any traversal attempt) is rejected with ErrPathNotAllowed.
// Hidden directories (starting with ".") are excluded from the listing.
//
// Used by the folder picker for repo-less tasks. Anchoring to the discovery
// roots keeps the endpoint from being a path-traversal foothold even though
// it runs locally — kandev has no business reading arbitrary host paths.
func (s *Service) ListDirectory(ctx context.Context, path string) (DirectoryListing, error) {
	roots := s.discoveryRoots()
	abs, err := resolveListingPath(path, roots)
	if err != nil {
		return DirectoryListing{}, err
	}

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

	out := collectSubdirs(abs, entries)

	_ = ctx
	return DirectoryListing{
		Path:    abs,
		Parent:  parentWithinRoots(abs, roots),
		Entries: out,
	}, nil
}

// resolveListingPath cleans the user-supplied path and verifies it sits
// inside an allowed discovery root. Empty path defaults to the first root.
// Both inputs are normalized via filepath.Abs + filepath.Clean before the
// containment check, so ".." or symlink-style traversal can't escape.
func resolveListingPath(path string, roots []string) (string, error) {
	if len(roots) == 0 {
		return "", fmt.Errorf("no allowed roots configured")
	}
	if path == "" {
		return filepath.Clean(roots[0]), nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	abs = filepath.Clean(abs)
	if !isPathAllowed(abs, roots) {
		return "", ErrPathNotAllowed
	}
	return abs, nil
}

// parentWithinRoots returns the parent of abs, or "" when abs is at or
// above an allowed root (so the picker's "up" button stops at the root and
// doesn't offer to navigate outside the allowed subtree).
func parentWithinRoots(abs string, roots []string) string {
	parent := filepath.Dir(abs)
	if parent == abs {
		return ""
	}
	if !isPathAllowed(parent, roots) {
		return ""
	}
	return parent
}

// collectSubdirs filters entries to immediate subdirectories, drops hidden
// (dotfile) directories, and returns them sorted alphabetically (case-fold).
func collectSubdirs(parent string, entries []os.DirEntry) []DirectoryEntry {
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
			Path: filepath.Join(parent, name),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}
