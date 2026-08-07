package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
//
// The returned dbPath is display-only; the backend derives its own DB path
// from KANDEV_HOME_DIR via resolveDatabasePath(). Both resolve to the same
// location.
func resolveDevBackendEnv(repoRoot string) (dbPath string, extra []string) {
	devHome := devKandevHome(repoRoot)
	devDBPath := filepath.Join(devHome, "data", "kandev.db")

	// Profile-selector only: the backend reads profiles.yaml at startup and
	// applies the matching dev: values (mock agent, pprof, feature flags,
	// etc.) to its own env. The launcher must not restate them — profiles.yaml
	// at the repo root is the single source of truth.
	baseExtra := []string{"KANDEV_DEBUG_DEV_MODE=true"}

	if isInsideKandevTask(repoRoot) {
		fmt.Println("[kandev] task workspace detected → using local dev state")
		return devDBPath, append(baseExtra,
			"KANDEV_HOME_DIR="+devHome,
			// Clear a parent-leaked DB path so the backend uses the
			// HomeDir-derived default.
			"KANDEV_DATABASE_PATH=",
		)
	}

	if override := strings.TrimSpace(os.Getenv("KANDEV_DATABASE_PATH")); override != "" {
		return override, append(baseExtra, "KANDEV_DATABASE_PATH="+override)
	}

	return devDBPath, append(baseExtra,
		"KANDEV_HOME_DIR="+devHome,
		"KANDEV_DATABASE_PATH=",
	)
}
