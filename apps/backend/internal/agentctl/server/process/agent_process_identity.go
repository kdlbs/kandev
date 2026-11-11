package process

import (
	"github.com/kandev/kandev/internal/common/processidentity"
)

// recordAgentProcessIdentity captures the started agent child's process-group
// birth identity. Agents run with Setpgid, so the child leads its own group;
// any other shape is not recorded and recovery keeps the runtime-only proof.
func (m *Manager) recordAgentProcessIdentity(pid int) {
	identity, err := processidentity.Capture(pid)
	if err != nil || identity.Validate() != nil || identity.GroupID != identity.PID {
		m.agentProcessIdentity.Store(nil)
		return
	}
	m.agentProcessIdentity.Store(&identity)
}

// clearAgentProcessIdentity forgets the identity of an exited child.
func (m *Manager) clearAgentProcessIdentity(pid int) {
	if current := m.agentProcessIdentity.Load(); current != nil && current.PID == pid {
		m.agentProcessIdentity.CompareAndSwap(current, nil)
	}
}

// AgentProcessIdentity returns the running agent child's process-group
// identity, or nil when no verifiable child is running.
func (m *Manager) AgentProcessIdentity() *processidentity.Identity {
	if current := m.agentProcessIdentity.Load(); current != nil {
		identity := *current
		return &identity
	}
	return nil
}
