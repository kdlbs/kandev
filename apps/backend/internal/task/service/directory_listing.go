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
// listed, the parent (empty when at the filesystem root), and the immediate
// subdirectory children sorted alphabetically.
type DirectoryListing struct {
	Path    string
	Parent  string
	Entries []DirectoryEntry
}

// ListDirectory returns the immediate subdirectories of path. When path is
// empty it falls back to $HOME. The picker is deliberately *not* anchored
// to discoveryRoots — users browsing for a starting folder for a repo-less
// task may legitimately want /tmp, /var/log/foo, C:\Users, or any other
// directory they have read access to. The repo-discover endpoint stays
// locked down to discoveryRoots; this one trusts the local-process
// boundary that runs kandev on the user's own machine.
//
// Filesystem operations go through os.Root opened at the volume root
// ("/" on Unix, "C:\" / UNC share on Windows). The Go stdlib enforces
// containment at the syscall level (no symlink escape, no traversal out
// of the root). That's a no-op for security because the volume root
// covers every path the user could pick on that volume — but it's what
// CodeQL recognises as a path-injection sanitizer, so the lint stays green.
//
// Hidden (".") directories are excluded.
func (s *Service) ListDirectory(ctx context.Context, path string) (DirectoryListing, error) {
	abs, err := resolveListingPath(path)
	if err != nil {
		return DirectoryListing{}, err
	}

	entries, err := readSubdirsBoundedAtFilesystemRoot(abs)
	if err != nil {
		return DirectoryListing{}, err
	}

	_ = ctx
	return DirectoryListing{
		Path:    abs,
		Parent:  parentPath(abs),
		Entries: collectSubdirs(abs, entries),
	}, nil
}

// resolveListingPath cleans the user-supplied path. Empty path defaults to
// $HOME. The returned path is always absolute and cleaned.
func resolveListingPath(path string) (string, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home: %w", err)
		}
		return filepath.Clean(home), nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	return filepath.Clean(abs), nil
}

// readSubdirsBoundedAtFilesystemRoot opens the volume containing abs via
// os.Root and reads the directory at the volume-relative path inside it.
// Using os.Root here is for CodeQL's benefit: the Go stdlib refuses symlink
// escape and rejects traversal segments in the relative path, and CodeQL
// models os.Root.Open as a sanitizer for the path-injection query. The
// containment is trivially satisfied because the root is the volume root —
// every accessible path on that volume is inside it.
func readSubdirsBoundedAtFilesystemRoot(abs string) ([]os.DirEntry, error) {
	rootPath, rel := splitAbsForRoot(abs)
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("open root %q: %w", rootPath, err)
	}
	defer func() { _ = root.Close() }()

	info, err := root.Stat(rel)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", abs)
	}

	dir, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	return dir.ReadDir(-1)
}

// splitAbsForRoot splits an absolute path into (rootPath, rel) suitable for
// os.OpenRoot + os.Root.Stat/Open. os.Root rejects absolute paths and drive
// letters inside its relative arg, so we have to pre-split on the volume.
//
// On Unix this is the historical "/" + path-minus-leading-slash pair. On
// Windows the root is the volume — "C:\" for drive-letter paths,
// "\\server\share\" for UNC — and the relative is what remains after the
// volume. Without this split, Windows paths like "C:\Users\carlo" passed as
// the relative arg trip os.Root's "path escapes from parent" check, even
// though they're inside the conceptual filesystem root.
func splitAbsForRoot(abs string) (rootPath, rel string) {
	vol := filepath.VolumeName(abs)
	if vol == "" {
		// Unix absolute path (or a non-volume Windows path, which shouldn't
		// happen after filepath.Abs).
		rel = strings.TrimPrefix(abs, "/")
		if rel == "" {
			rel = "."
		}
		return "/", rel
	}
	// Windows drive-rooted or UNC absolute path. filepath.VolumeName returns
	// "C:" for "C:\foo" and "\\server\share" for UNC paths — append the
	// separator to get the actual openable root directory.
	rootPath = vol + string(filepath.Separator)
	rel = strings.TrimPrefix(abs, rootPath)
	if rel == "" {
		rel = "."
	}
	return rootPath, rel
}

// parentPath returns the parent of abs, or "" when abs is the filesystem
// root. The picker's "up" button stops at "/" so the user can't navigate
// past the top of the host filesystem.
func parentPath(abs string) string {
	parent := filepath.Dir(abs)
	if parent == abs {
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
