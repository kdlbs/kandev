package lifecycle

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/agents"
)

var (
	ErrOpenCodeMigrationUnavailable = errors.New("OpenCode migration admission is unavailable")
	ErrOpenCodeExecutionActive      = errors.New("OpenCode executions are active")
)

// AcquireOpenCodeMigration reserves the launch-admission boundary used for the
// authoritative runtime-selection write. It refuses active host work and any
// remaining tracked OpenCode execution, including executions on remote hosts.
func (m *Manager) AcquireOpenCodeMigration(ctx context.Context) (context.Context, func(), error) {
	if m == nil || m.executionStore == nil {
		return nil, nil, ErrOpenCodeMigrationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	m.openCodeAdmission.Lock()
	if err := ctx.Err(); err != nil {
		m.openCodeAdmission.Unlock()
		return nil, nil, err
	}
	for _, execution := range m.executionStore.List() {
		if execution.AgentID == agents.OpenCodeACPAgentID {
			m.openCodeAdmission.Unlock()
			return nil, nil, ErrOpenCodeExecutionActive
		}
	}
	return ctx, m.openCodeAdmission.Unlock, nil
}

func (m *Manager) acquireOpenCodeLaunchAdmission(agentType string) func() {
	if agentType != agents.OpenCodeACPAgentID {
		return func() {}
	}
	m.openCodeAdmission.RLock()
	return m.openCodeAdmission.RUnlock
}
