package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// AgentctlResolver finds the path to a linux/amd64 agentctl binary for remote executors.
// Resolution order:
//  1. KANDEV_AGENTCTL_LINUX_BINARY env var
//  2. build/agentctl-linux-amd64 relative to the running binary (dev mode)
//  3. bin/agentctl-linux-amd64 relative to the running binary
type AgentctlResolver struct {
	logger *logger.Logger
}

// NewAgentctlResolver creates a new resolver.
func NewAgentctlResolver(log *logger.Logger) *AgentctlResolver {
	return &AgentctlResolver{
		logger: log.WithFields(zap.String("component", "agentctl_resolver")),
	}
}

// ResolveLinuxBinary returns the path to a linux/amd64 agentctl binary.
func (r *AgentctlResolver) ResolveLinuxBinary() (string, error) {
	// 1. Env var override
	if envPath := os.Getenv("KANDEV_AGENTCTL_LINUX_BINARY"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			r.logger.Debug("using agentctl from env var", zap.String("path", envPath))
			return envPath, nil
		}
		return "", fmt.Errorf("KANDEV_AGENTCTL_LINUX_BINARY=%q does not exist", envPath)
	}

	// 2. Relative to the running binary (dev mode: build/agentctl-linux-amd64)
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidates := []string{
			filepath.Join(exeDir, "agentctl-linux-amd64"),
			filepath.Join(exeDir, "..", "build", "agentctl-linux-amd64"),
			filepath.Join(exeDir, "..", "bin", "agentctl-linux-amd64"),
		}
		for _, candidate := range candidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				abs, _ := filepath.Abs(candidate)
				r.logger.Debug("found agentctl binary", zap.String("path", abs))
				return abs, nil
			}
		}
	}

	return "", fmt.Errorf(
		"agentctl linux binary not found; build it with 'make build-agentctl-linux' "+
			"or set KANDEV_AGENTCTL_LINUX_BINARY (os=%s arch=%s)",
		runtime.GOOS, runtime.GOARCH,
	)
}
