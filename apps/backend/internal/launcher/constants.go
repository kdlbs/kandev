package launcher

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultBackendPort  = 38429
	defaultWebPort      = 37429
	defaultAgentctlPort = 39429
	parentPIDEnv        = "KANDEV_LAUNCHER_PARENT_PID"

	healthTimeoutReleaseMS = 45000
	healthTimeoutDevMS     = 600000
	randomPortMin          = 10000
	randomPortMax          = 60000

	// devKandevDotdir roots all `make dev` state inside the repo so dev mode
	// never mutates the user's production ~/.kandev and so `make clean-db`
	// (which removes .kandev-dev/) matches what `make dev` writes.
	devKandevDotdir = ".kandev-dev"
)

func resolveHomeDir() string {
	if v := strings.TrimSpace(os.Getenv("KANDEV_HOME_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".kandev"
	}
	return filepath.Join(home, ".kandev")
}

func resolveDataDir() string {
	return filepath.Join(resolveHomeDir(), "data")
}

func resolveDatabasePath() string {
	if v := strings.TrimSpace(os.Getenv("KANDEV_DATABASE_PATH")); v != "" {
		return v
	}
	return filepath.Join(resolveDataDir(), "kandev.db")
}

func devKandevHome(repoRoot string) string {
	return filepath.Join(repoRoot, devKandevDotdir)
}

// userHomeDir returns os.UserHomeDir() with a relative fallback, mirroring
// resolveHomeDir's error handling without consulting KANDEV_HOME_DIR.
func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// kandevTasksDir locates the task-workspace root. The real user home is used
// rather than the ambient KANDEV_HOME_DIR: inside a task workspace that env
// var belongs to the parent backend, and the tasks directory always lives
// under the user's own home regardless of any per-process home override.
func kandevTasksDir() string {
	return filepath.Join(userHomeDir(), ".kandev", "tasks")
}
