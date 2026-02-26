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

	if os.Getenv("KANDEV_MOCK_AGENT") == "true" {
		// Only register mock agent — skip slow agent discovery for all others
		_ = reg.Register(agents.NewMockAgent())
		configureMockAgent(reg, log)
	} else {
		reg.LoadDefaults()
	}

	return reg, func() error { return nil }, nil
}

// configureMockAgent enables and configures the mock agent binary path and capabilities.
// KANDEV_MOCK_AGENT_MCP=false disables MCP support (defaults to enabled).
func configureMockAgent(reg *Registry, log *logger.Logger) {
	ag, ok := reg.Get("mock-agent")
	if !ok {
		return
	}
	mock, ok := ag.(*agents.MockAgent)
	if !ok {
		return
	}
	mock.SetEnabled(true)
	if os.Getenv("KANDEV_MOCK_AGENT_MCP") == "false" {
		mock.SetSupportsMCP(false)
	}
	// Resolve binary path: same directory as the running executable
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	binaryPath := filepath.Join(filepath.Dir(exePath), "mock-agent")
	mock.SetBinaryPath(binaryPath)
	log.Info("mock agent enabled",
		zap.String("cmd", binaryPath),
		zap.Bool("supports_mcp", mock.SupportsMCPEnabled()))
}
