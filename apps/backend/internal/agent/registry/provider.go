package registry

import (
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Provide creates and loads the agent registry.
func Provide(log *logger.Logger) (*Registry, func() error, error) {
	reg := NewRegistry(log)
	reg.LoadDefaults()

	if os.Getenv("KANDEV_MOCK_AGENT") == "true" {
		if ag, ok := reg.Get("mock-agent"); ok {
			if mock, ok := ag.(*agents.MockAgent); ok {
				mock.SetEnabled(true)
				// Resolve binary path: same directory as the running executable
				if exePath, err := os.Executable(); err == nil {
					binaryPath := filepath.Join(filepath.Dir(exePath), "mock-agent")
					mock.SetBinaryPath(binaryPath)
					log.Info("mock agent enabled", zap.String("cmd", binaryPath))
				}
			}
		}
	}

	return reg, func() error { return nil }, nil
}
