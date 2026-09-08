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
	path    string
	homeDir string
	source  devDatabaseSource
	extra   []string
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
	homeDir := devStateHome(repoRoot, startupConfig)

	if isInsideKandevTask(repoRoot) {
		fmt.Println("[kandev] task workspace detected → using local dev state")
		return newDevDatabaseTarget(devDBPath, devHome, devDatabaseTask, baseExtra)
	}
	if startupConfig == nil {
		if override := strings.TrimSpace(os.Getenv("KANDEV_DATABASE_PATH")); override != "" {
			return newDevDatabaseTarget(override, homeDir, devDatabaseEnvironment, baseExtra)
		}
	}

	if startupConfig != nil && strings.TrimSpace(startupConfig.Database.Path) != "" {
		switch startupConfig.SourceFor("database.path") {
		case config.SourceEnvironment:
			return newDevDatabaseTarget(startupConfig.Database.Path, homeDir, devDatabaseEnvironment, baseExtra)
		case config.SourceConfiguration:
			return newDevDatabaseTarget(startupConfig.Database.Path, homeDir, devDatabaseConfiguration, baseExtra)
		}
	}
	if startupConfig != nil && startupConfig.SourceFor("homeDir") == config.SourceConfiguration {
		path := filepath.Join(startupConfig.ResolvedDataDir(), "kandev.db")
		return newDevDatabaseTarget(path, homeDir, devDatabaseConfigHome, baseExtra)
	}

	return newDevDatabaseTarget(devDBPath, homeDir, devDatabaseDefault, baseExtra)
}

func devStateHome(repoRoot string, startupConfig *config.Config) string {
	if startupConfig != nil && startupConfig.SourceFor("homeDir") == config.SourceConfiguration {
		return startupConfig.ResolvedHomeDir()
	}
	return devKandevHome(repoRoot)
}

func newDevDatabaseTarget(path, homeDir string, source devDatabaseSource, baseExtra []string) devDatabaseTarget {
	extra := append([]string{}, baseExtra...)
	extra = append(extra, "KANDEV_HOME_DIR="+homeDir, "KANDEV_DATABASE_PATH="+path)
	return devDatabaseTarget{path: path, homeDir: homeDir, source: source, extra: extra}
}

// validateDevDatabaseTarget checks whether the resolved database target is
// safe to use in dev mode. Repo-local targets must live under
// .kandev-dev/data/ and must not traverse symlinks. Repo-local-default or
// task targets that land outside the repo-local state are rejected outright.
// External targets (environment or YAML) are allowed without symlink
// checking; they may be the user's explicit production database.
func validateDevDatabaseTarget(repoRoot string, target devDatabaseTarget) error {
	path, err := filepath.Abs(target.path)
	if err != nil {
		return fmt.Errorf("resolve dev database path %q: %w", target.path, err)
	}
	devHome, err := filepath.Abs(devKandevHome(repoRoot))
	if err != nil {
		return fmt.Errorf("resolve dev home directory: %w", err)
	}
	dataDir, err := filepath.Abs(filepath.Join(devKandevHome(repoRoot), "data"))
	if err != nil {
		return fmt.Errorf("resolve dev data directory: %w", err)
	}
	insideDataDir := pathWithinDirectory(dataDir, path)
	insideDevHome := pathWithinDirectory(devHome, path)
	if (insideDevHome && !insideDataDir) || (!insideDataDir && (target.source == devDatabaseDefault || target.source == devDatabaseTask)) {
		return fmt.Errorf("refusing dev database path outside repo-local data directory: %s", path)
	}
	if insideDataDir {
		if err := rejectSymlinkComponents(path); err != nil {
			return fmt.Errorf("refusing dev database path %s: %w", path, err)
		}
	}
	return nil
}

func pathWithinDirectory(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// normalizeDevDatabaseTarget sets the target path and homeDir to their
// absolute forms and updates the corresponding entries in extra so all
// consumers (backup, output, child environment, supervisor paths) resolve
// them consistently regardless of the launcher's CWD.
func normalizeDevDatabaseTarget(target devDatabaseTarget) (devDatabaseTarget, error) {
	absPath, err := filepath.Abs(target.path)
	if err != nil {
		return devDatabaseTarget{}, fmt.Errorf("resolve database path: %w", err)
	}
	target.path = absPath

	absHome, err := filepath.Abs(target.homeDir)
	if err != nil {
		return devDatabaseTarget{}, fmt.Errorf("resolve home directory: %w", err)
	}
	target.homeDir = absHome

	for i, item := range target.extra {
		switch {
		case strings.HasPrefix(item, "KANDEV_DATABASE_PATH="):
			target.extra[i] = "KANDEV_DATABASE_PATH=" + absPath
		case strings.HasPrefix(item, "KANDEV_HOME_DIR="):
			target.extra[i] = "KANDEV_HOME_DIR=" + absHome
		}
	}
	return target, nil
}
