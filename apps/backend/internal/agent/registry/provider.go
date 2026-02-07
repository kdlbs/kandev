package registry

import (
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Provide creates and loads the agent registry.
func Provide(log *logger.Logger) (*Registry, func() error, error) {
	reg := NewRegistry(log)
	reg.LoadDefaults()

	if os.Getenv("KANDEV_MOCK_AGENT") == "true" {
		if cfg, ok := reg.Get("mock-agent"); ok {
			cfg.Enabled = true
			// Resolve binary path: same directory as the running executable
			if exePath, err := os.Executable(); err == nil {
				cfg.Cmd = []string{filepath.Join(filepath.Dir(exePath), "mock-agent")}
			}
			log.Info("mock agent enabled", zap.String("cmd", cfg.Cmd[0]))
		}
	}

	return reg, func() error { return nil }, nil
}
