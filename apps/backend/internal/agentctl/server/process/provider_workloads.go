package process

import (
	"github.com/kandev/kandev/internal/agentctl/server/process/probe"
	"time"
)

func (m *Manager) ProviderBackgroundWorkloads(turnStart time.Time) (probe.Result, error) {
	return probe.ProbeProviderBackgroundWorkloads(m.AgentPID(), turnStart, m.ProcessManagerID())
}
