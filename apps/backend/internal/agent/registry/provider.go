package registry

import (
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Provide creates and loads the agent registry.
//
// KANDEV_MOCK_AGENT controls mock-agent availability:
//   - "only"  → E2E mode: only register mock agent, skip all others
//   - "true"  → Dev mode: load all agents AND enable mock agent
//   - unset   → Production: load all agents, mock agent disabled
func Provide(log *logger.Logger) (*Registry, func() error, error) {
	reg := NewRegistry(log)

	mockMode := os.Getenv("KANDEV_MOCK_AGENT")
	if mockMode == "only" {
		// E2E mode: only register mock agent — skip agent discovery for all others
		_ = reg.Register(agents.NewMockAgent())
		configureMockAgent(reg, log)
	} else {
		reg.LoadDefaults()
		if mockMode == "true" {
			// Dev mode: enable mock agent alongside all other agents
			configureMockAgent(reg, log)
		}
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
