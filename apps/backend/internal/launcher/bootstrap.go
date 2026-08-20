package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
)

func loadBootstrapConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load startup configuration: %w", err)
	}
	return cfg, nil
}

func configSourceIsExplicit(cfg *config.Config, key string) bool {
	if cfg == nil {
		return false
	}
	source := cfg.SourceFor(key)
	return source == config.SourceConfiguration || source == config.SourceEnvironment || source == config.SourceProfile
}

func resolveHomeDirForConfig(cfg *config.Config) string {
	if cfg == nil {
		return resolveHomeDir()
	}
	return cfg.ResolvedHomeDir()
}

func resolveDataDirForConfig(cfg *config.Config) string {
	if cfg == nil {
		return resolveDataDir()
	}
	return cfg.ResolvedDataDir()
}

func resolveDatabasePathForConfig(cfg *config.Config) string {
	if cfg == nil {
		return resolveDatabasePath()
	}
	if path := strings.TrimSpace(cfg.Database.Path); path != "" {
		return path
	}
	return filepath.Join(resolveDataDirForConfig(cfg), "kandev.db")
}

func ensureDataDirForConfig(cfg *config.Config) error {
	dir := resolveDataDirForConfig(cfg)
	if err := rejectSymlinkComponents(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return nil
}

func configFileForChild(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Source.FilePath
}
