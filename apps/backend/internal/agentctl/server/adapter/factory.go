package adapter

import (
	"fmt"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// NewAdapter creates a new protocol adapter based on the specified protocol type.
// It returns an error if the protocol is not supported.
func NewAdapter(protocol agent.Protocol, cfg *Config, log *logger.Logger) (AgentAdapter, error) {
	switch protocol {
	case agent.ProtocolACP:
		return NewACPAdapter(cfg, log), nil
	case agent.ProtocolCodex:
		return NewCodexAdapter(cfg, log), nil
	case agent.ProtocolClaudeCode:
		return NewClaudeCodeAdapter(cfg, log), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}
