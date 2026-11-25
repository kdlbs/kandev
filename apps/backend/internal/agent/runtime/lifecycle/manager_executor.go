package lifecycle

import (
	"github.com/kandev/kandev/internal/agent/executor"
)

// RegisterExecutorBackend adds a runtime backend before the manager starts.
func (m *Manager) RegisterExecutorBackend(backend ExecutorBackend) {
	if backend == nil || m.executorRegistry == nil {
		return
	}
	m.executorRegistry.Register(backend)
}

// getExecutorBackend returns the backend required by the selection.
//
// A nonempty selection is always required: an unregistered backend is a
// refusal, never a fallback to standalone. The ExecutorFallback* policy values
// are retained only for internal source compatibility with existing wiring;
// they no longer authorize replacing a concrete selection. An empty selection
// keeps the existing standalone default.
func (m *Manager) getExecutorBackend(executorType string) (ExecutorBackend, error) {
	return m.requireRuntimeBackend(executorType)
}

// getDefaultExecutorBackend returns the default runtime (standalone).
// This is used when no executor type is specified.
func (m *Manager) getDefaultExecutorBackend() (ExecutorBackend, error) {
	if m.executorRegistry == nil {
		return nil, &ExecutorRuntimeError{
			Reason: ExecutorRuntimeReasonUnavailable,
			Detail: "runtime registry is not configured",
		}
	}
	return m.executorRegistry.GetBackend(executor.NameStandalone)
}
