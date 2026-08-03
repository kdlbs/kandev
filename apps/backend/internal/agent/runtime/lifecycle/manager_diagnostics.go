package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/subproc"
)

type subprocessAdmissionProvider interface {
	SubprocessAdmission(context.Context) (subproc.Snapshot, error)
}

// SubprocessAdmission returns the standalone agentctl admission snapshot when
// the configured default runtime exposes it. Other runtimes remain optional:
// callers can still export the backend-local snapshot when this probe fails.
func (m *Manager) SubprocessAdmission(ctx context.Context) (subproc.Snapshot, error) {
	if m.executorRegistry == nil {
		return subproc.Snapshot{}, fmt.Errorf("no runtime registry configured")
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameStandalone)
	if err != nil {
		return subproc.Snapshot{}, err
	}
	provider, ok := backend.(subprocessAdmissionProvider)
	if !ok {
		return subproc.Snapshot{}, fmt.Errorf("standalone runtime does not expose subprocess admission")
	}
	return provider.SubprocessAdmission(ctx)
}
