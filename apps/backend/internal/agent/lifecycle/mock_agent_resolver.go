package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// MockAgentResolver finds the path to a linux/amd64 mock-agent binary so it
// can be bind-mounted into Docker containers for E2E tests. Resolution order:
//  1. KANDEV_MOCK_AGENT_LINUX_BINARY env var
//  2. build/mock-agent-linux-amd64 relative to the running binary (dev mode)
//
// Unlike AgentctlResolver, "not found" is not an error — only the Docker E2E
// suite needs this binary, so production paths get a nil mount silently.
type MockAgentResolver struct {
	logger *logger.Logger
}

// NewMockAgentResolver creates a new resolver.
func NewMockAgentResolver(log *logger.Logger) *MockAgentResolver {
	return &MockAgentResolver{
		logger: log.WithFields(zap.String("component", "mock_agent_resolver")),
	}
}

// ResolveLinuxBinary returns the path to a linux/amd64 mock-agent binary, or
// ("", nil) when none is configured. Returns an error only when an explicit
// env var path is invalid.
func (r *MockAgentResolver) ResolveLinuxBinary() (string, error) {
	if envPath := os.Getenv("KANDEV_MOCK_AGENT_LINUX_BINARY"); envPath != "" {
		info, err := os.Stat(envPath)
		if err != nil {
			return "", fmt.Errorf("KANDEV_MOCK_AGENT_LINUX_BINARY=%q does not exist", envPath)
		}
		// Reject directories (and anything that isn't a regular file) up
		// front. Otherwise the path slips through to the bind-mount + exec
		// step and surfaces as a less actionable "permission denied" or
		// "exec format error" deep inside container startup.
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("KANDEV_MOCK_AGENT_LINUX_BINARY=%q is not a regular file", envPath)
		}
		r.logger.Debug("using mock-agent from env var", zap.String("path", envPath))
		return envPath, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", nil
	}
	exeDir := filepath.Dir(exePath)
	candidates := []string{
		filepath.Join(exeDir, "mock-agent-linux-amd64"),
		filepath.Join(exeDir, "..", "build", "mock-agent-linux-amd64"),
		filepath.Join(exeDir, "..", "bin", "mock-agent-linux-amd64"),
	}
	for _, candidate := range candidates {
		if _, statErr := os.Stat(candidate); statErr == nil {
			abs, _ := filepath.Abs(candidate)
			r.logger.Debug("found mock-agent binary", zap.String("path", abs))
			return abs, nil
		}
	}
	return "", nil
}
