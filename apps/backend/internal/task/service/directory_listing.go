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
// $HOME). Filesystem operations are bounded to the matching discovery root
// via os.Root, so symlinks and path traversal cannot escape the configured
// allowlist even at the OS level. Hidden directories (starting with ".")
// are excluded from the listing.
//
// Used by the folder picker for repo-less tasks.
func (s *Service) ListDirectory(ctx context.Context, path string) (DirectoryListing, error) {
	roots := s.discoveryRoots()
	if len(roots) == 0 {
		return DirectoryListing{}, fmt.Errorf("no allowed roots configured")
	}
	target, err := resolveListingTarget(path, roots)
	if err != nil {
		return DirectoryListing{}, err
	}

	entries, err := readSubdirsBoundedByRoot(target.root, target.rel)
	if err != nil {
		return DirectoryListing{}, err
	}

	_ = ctx
	return DirectoryListing{
		Path:    target.abs,
		Parent:  parentWithinRoots(target.abs, roots),
		Entries: collectSubdirs(target.abs, entries),
	}, nil
}

// listingTarget describes the resolved listing target as a (root, rel)
// pair. abs is the resolved absolute path (root + rel) — kept for the
// response payload and breadcrumb display.
type listingTarget struct {
	root string // discovery root that contains the target
	rel  string // path relative to root (cleaned, no traversal segments)
	abs  string
}

// resolveListingTarget normalises the user-supplied path, picks the matching
// discovery root, and returns the (root, rel, abs) triple. Empty path falls
// back to the first root.
func resolveListingTarget(path string, roots []string) (listingTarget, error) {
	if path == "" {
		root := filepath.Clean(roots[0])
		return listingTarget{root: root, rel: ".", abs: root}, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return listingTarget{}, fmt.Errorf("invalid path: %w", err)
	}
	abs = filepath.Clean(abs)
	for _, r := range roots {
		root := filepath.Clean(r)
		if !isWithinRoot(abs, root) {
			continue
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			continue
		}
		// Defensive: filepath.Rel on a contained path never returns a
		// traversal-bearing rel, but reject if it ever did.
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		return listingTarget{root: root, rel: rel, abs: abs}, nil
	}
	return listingTarget{}, ErrPathNotAllowed
}

// readSubdirsBoundedByRoot opens the discovery root via os.Root and reads
// the directory at `rel`. os.Root enforces containment at the OS level —
// symlinks pointing outside the root are refused, and any traversal in
// `rel` returns an error. This is the canonical Go 1.24+ sanitizer for
// "user-supplied path inside a trusted root".
func readSubdirsBoundedByRoot(root, rel string) ([]os.DirEntry, error) {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}
	defer func() { _ = rootFS.Close() }()

	info, err := rootFS.Stat(rel)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", rel)
	}

	dir, err := rootFS.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	return dir.ReadDir(-1)
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
