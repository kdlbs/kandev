// Package disk computes a lazy, cached breakdown of the on-disk kandev data
// footprint (data dir, worktrees, repos, sessions, tasks, quick-chat, and
// backups) for the System -> Status page. The walk is tolerant of permission
// errors and surfaces them as Warnings on the returned Breakdown.
package disk

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// subdir describes a logical bucket of the breakdown — its display name (the
// JSON / breakdown field) and the absolute path on disk it covers.
type subdir struct {
	name string
	path string
}

// subdirsFor returns the list of subdirectories that contribute to the
// breakdown for a given home directory. The backups subdir lives under
// <home>/data/backups; the rest are direct children of <home>.
func subdirsFor(homeDir string) []subdir {
	return []subdir{
		{name: "data_dir", path: filepath.Join(homeDir, "data")},
		{name: "worktrees", path: filepath.Join(homeDir, "worktrees")},
		{name: "repos", path: filepath.Join(homeDir, "repos")},
		{name: "sessions", path: filepath.Join(homeDir, "sessions")},
		{name: "tasks", path: filepath.Join(homeDir, "tasks")},
		{name: "quick_chat", path: filepath.Join(homeDir, "quick-chat")},
		{name: "backups", path: filepath.Join(homeDir, "data", "backups")},
	}
}

// walkResult is the per-subdir output of walkSubdir.
type walkResult struct {
	bytes    int64
	warnings []string
}

// walkSubdir recursively sums regular file sizes under root. Permission
// errors on subdirectories are recorded into warnings and the walk
// continues with the next sibling. A missing root (os.IsNotExist) yields a
// zero-byte result with no warning — empty installs are normal.
func walkSubdir(root string) walkResult {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return walkResult{}
		}
		return walkResult{warnings: []string{fmt.Sprintf("%s: %s", root, err)}}
	}
	if !info.IsDir() {
		// Treat a regular file as its own size; uncommon but cheap to handle.
		return walkResult{bytes: info.Size()}
	}

	var result walkResult
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.warnings = append(result.warnings, fmt.Sprintf("%s: %s", path, walkErr))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, infoErr := d.Info()
		if infoErr != nil {
			result.warnings = append(result.warnings, fmt.Sprintf("%s: %s", path, infoErr))
			return nil
		}
		if fi.Mode().IsRegular() {
			result.bytes += fi.Size()
		}
		return nil
	})
	if err != nil {
		result.warnings = append(result.warnings, fmt.Sprintf("%s: %s", root, err))
	}
	return result
}
