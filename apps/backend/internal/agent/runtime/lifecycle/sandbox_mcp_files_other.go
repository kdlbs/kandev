//go:build !linux

package lifecycle

import (
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (m *Manager) materializeSandboxMCPFileLocked(_ *AgentExecution, _ mcpconfig.PassthroughConfigFile) (bool, error) {
	return false, contract.ErrInvalidPolicy
}

func (m *Manager) cleanupSandboxMCPClaimLocked(_ *AgentExecution, _ passthroughMCPFileClaim) {}
