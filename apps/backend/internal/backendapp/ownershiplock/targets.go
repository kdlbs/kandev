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
	if !strings.EqualFold(databaseDriver, "sqlite") || strings.TrimSpace(databasePath) == "" {
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

	// The resource itself may not exist yet, but an existing symlinked parent
	// still needs to resolve to the same lock identity as its real path.
	parent := filepath.Dir(abs)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err == nil {
		return filepath.Join(filepath.Clean(resolvedParent), filepath.Base(abs)), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return abs, nil
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
