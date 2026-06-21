package launcher

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultBackendPort  = 38429
	defaultAgentctlPort = 39429

	healthTimeoutReleaseMS = 45000
	randomPortMin          = 10000
	randomPortMax          = 60000
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
