package ownershiplock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// TargetKind identifies the persistent resource protected by an ownership lock.
type TargetKind string

const (
	TargetHome     TargetKind = "home"
	TargetDatabase TargetKind = "database"
)

// Target describes the canonical resource path and the stable sidecar file used
// to hold its operating-system lock.
type Target struct {
	Kind         TargetKind
	ResourcePath string
	LockPath     string
}

// Targets returns the persistent resources a backend must own before startup.
// The Kandev home owns the default SQLite database, so an explicit database
// target is needed only when SQLite points outside that home.
func Targets(homeDir, databaseDriver, databasePath string) ([]Target, error) {
	home, err := canonicalPath(homeDir)
	if err != nil {
		return nil, fmt.Errorf("canonicalize Kandev home %q: %w", homeDir, err)
	}
	if home == "" {
		return nil, errors.New("kandev home cannot be empty")
	}

	targets := []Target{{
		Kind:         TargetHome,
		ResourcePath: home,
		LockPath:     filepath.Join(home, ".kandev-backend.lock"),
	}}
	if !strings.EqualFold(databaseDriver, "sqlite") {
		return targets, nil
	}
	if strings.TrimSpace(databasePath) == "" {
		// Empty path uses the in-home SQLite database, already covered by the
		// home lock.
		return targets, nil
	}

	database, err := canonicalPath(databasePath)
	if err != nil {
		return nil, fmt.Errorf("canonicalize SQLite database %q: %w", databasePath, err)
	}
	if pathWithin(home, database) {
		return targets, nil
	}

	return append(targets, Target{
		Kind:         TargetDatabase,
		ResourcePath: database,
		LockPath:     database + ".lock",
	}), nil
}

func canonicalPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	// The resource itself may not exist yet, but every existing ancestor still
	// needs to resolve to the same lock identity as its real path. This also
	// normalizes Windows short-name aliases when a database directory is new.
	for parent := filepath.Dir(abs); ; parent = filepath.Dir(parent) {
		resolvedParent, err := filepath.EvalSymlinks(parent)
		if err == nil {
			relative, relErr := filepath.Rel(parent, abs)
			if relErr != nil {
				return "", relErr
			}
			return filepath.Clean(filepath.Join(resolvedParent, relative)), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return abs, nil
		}
	}
}

func pathWithin(base, candidate string) bool {
	if runtime.GOOS == "windows" {
		base = strings.ToLower(base)
		candidate = strings.ToLower(candidate)
	}
	relative, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
