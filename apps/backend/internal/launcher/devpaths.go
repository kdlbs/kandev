package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
)

// findRepoRoot returns the nearest ancestor of startDir that contains both
// apps/backend and apps/web, also handling a start directory that *is*
// <repo>/apps (dev is commonly invoked from apps/). It returns an error, not
// a fallback path, when no such ancestor exists: silently running dev against
// the wrong tree would be worse than failing loudly.
func findRepoRoot(startDir string) (string, error) {
	current := filepath.Clean(startDir)
	for {
		if filepath.Base(current) == "apps" &&
			exists(filepath.Join(current, "backend")) &&
			exists(filepath.Join(current, "web")) {
			return filepath.Dir(current), nil
		}
		if exists(filepath.Join(current, "apps", "backend")) &&
			exists(filepath.Join(current, "apps", "web")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf(
				"unable to locate repo root for dev (no ancestor of %s contains apps/backend and apps/web); run from the repository", startDir)
		}
		current = parent
	}
}

// isInsideKandevTask reports whether the current process looks like it was
// spawned inside a kandev-created task workspace. Two signals:
//  1. The parent kandev backend exports KANDEV_TASK_ID into every task shell.
//  2. Task worktrees live under ~/.kandev/tasks/.
//
// The path-prefix fallback is a defensive secondary signal for nested shells
// where KANDEV_TASK_ID was stripped. It is case-sensitive and does not
// resolve symlinks, so a realpath'd repoRoot may miss a symlinked HOME on
// macOS/Windows. KANDEV_TASK_ID remains the primary guarantee.
func isInsideKandevTask(repoRoot string) bool {
	if os.Getenv("KANDEV_TASK_ID") != "" {
		return true
	}
	return strings.HasPrefix(repoRoot, kandevTasksDir()+string(os.PathSeparator))
}

// resolveDevBackendEnv computes the dev-mode backend env. Dev mode always
// roots kandev under <repo>/.kandev-dev so state is isolated from the user's
// production ~/.kandev and so `make clean-db` (which removes .kandev-dev/)
// matches what `make dev` writes.
//
// When invoked from inside a kandev task workspace, any KANDEV_DATABASE_PATH
// is assumed to be leaked from the parent backend and is ignored. In a normal
// shell, an explicit KANDEV_DATABASE_PATH is honored as an escape hatch.
type devDatabaseSource string

const (
	devDatabaseDefault       devDatabaseSource = "repo-local default"
	devDatabaseTask          devDatabaseSource = "task workspace"
	devDatabaseEnvironment   devDatabaseSource = "environment"
	devDatabaseConfiguration devDatabaseSource = "configuration"
	devDatabaseConfigHome    devDatabaseSource = "configuration home"
)

// devDatabaseTarget is the one database decision shared by the launcher and
// its backend child. Keeping the provenance with the path prevents a backup
// from being performed against a different database than the child receives.
type devDatabaseTarget struct {
	path   string
	source devDatabaseSource
	extra  []string
}

func resolveDevBackendEnv(repoRoot string, configs ...*config.Config) (dbPath string, extra []string) {
	target := resolveDevDatabaseTarget(repoRoot, configs...)
	return target.path, target.extra
}

func resolveDevDatabaseTarget(repoRoot string, configs ...*config.Config) devDatabaseTarget {
	devHome := devKandevHome(repoRoot)
	devDBPath := filepath.Join(devHome, "data", "kandev.db")
	var startupConfig *config.Config
	if len(configs) > 0 {
		startupConfig = configs[0]
	}

	// Profile-selector only: the backend reads profiles.yaml at startup and
	// applies the matching dev: values (mock agent, pprof, feature flags,
	// etc.) to its own env. The launcher must not restate them — profiles.yaml
	// at the repo root is the single source of truth.
	baseExtra := []string{"KANDEV_DEBUG_DEV_MODE=true"}

	if isInsideKandevTask(repoRoot) {
		fmt.Println("[kandev] task workspace detected → using local dev state")
		return devDatabaseTarget{path: devDBPath, source: devDatabaseTask, extra: append(baseExtra,
			"KANDEV_HOME_DIR="+devHome,
			"KANDEV_DATABASE_PATH="+devDBPath,
		)}
	}
	if startupConfig == nil {
		if override := strings.TrimSpace(os.Getenv("KANDEV_DATABASE_PATH")); override != "" {
			return devDatabaseTarget{path: override, source: devDatabaseEnvironment, extra: append(baseExtra, "KANDEV_DATABASE_PATH="+override)}
		}
	}

	if startupConfig != nil && strings.TrimSpace(startupConfig.Database.Path) != "" {
		switch startupConfig.SourceFor("database.path") {
		case config.SourceEnvironment:
			return devDatabaseTarget{path: startupConfig.Database.Path, source: devDatabaseEnvironment, extra: append(baseExtra, "KANDEV_DATABASE_PATH="+startupConfig.Database.Path)}
		case config.SourceConfiguration:
			return devDatabaseTarget{path: startupConfig.Database.Path, source: devDatabaseConfiguration, extra: append(baseExtra, "KANDEV_DATABASE_PATH="+startupConfig.Database.Path)}
		}
	}
	if startupConfig != nil && startupConfig.SourceFor("homeDir") == config.SourceConfiguration {
		path := filepath.Join(startupConfig.ResolvedDataDir(), "kandev.db")
		return devDatabaseTarget{path: path, source: devDatabaseConfigHome, extra: append(baseExtra, "KANDEV_DATABASE_PATH="+path)}
	}

	return devDatabaseTarget{path: devDBPath, source: devDatabaseDefault, extra: append(baseExtra,
		"KANDEV_HOME_DIR="+devHome,
		"KANDEV_DATABASE_PATH="+devDBPath,
	)}
}

func validateDevDatabaseTarget(repoRoot string, target devDatabaseTarget) error {
	path, err := filepath.Abs(target.path)
	if err != nil {
		return fmt.Errorf("resolve dev database path %q: %w", target.path, err)
	}
	if target.source != devDatabaseDefault && target.source != devDatabaseTask {
		return nil
	}
	home, err := filepath.Abs(devKandevHome(repoRoot))
	if err != nil {
		return fmt.Errorf("resolve dev state home: %w", err)
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("refusing dev database path outside repo-local state: %s", path)
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return fmt.Errorf("refusing dev database path %s: %w", path, err)
	}
	return nil
}
