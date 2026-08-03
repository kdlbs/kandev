package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/subproc"
)

type admissionExecutor struct {
	MockExecutor
	snapshot subproc.Snapshot
}

func (e *admissionExecutor) SubprocessAdmission(context.Context) (subproc.Snapshot, error) {
	return e.snapshot, nil
}

func TestManagerSubprocessAdmissionUsesStandaloneProvider(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&admissionExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		snapshot:     subproc.Snapshot{Pool: "git", Capacity: 5},
	})
	manager := NewManager(nil, nil, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)

	snapshot, err := manager.SubprocessAdmission(context.Background())
	if err != nil {
		t.Fatalf("SubprocessAdmission() error = %v", err)
	}
	if snapshot.Pool != "git" || snapshot.Capacity != 5 {
		t.Fatalf("snapshot = %+v, want provider snapshot", snapshot)
	}
}
