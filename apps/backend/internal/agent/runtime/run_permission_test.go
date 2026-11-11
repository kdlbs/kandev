package runtime_test

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
)

type runPermissionBackend struct {
	*fakeBackend
	resolved []string
	canceled []string
}

func (b *runPermissionBackend) ResolveRunPermission(
	_ context.Context, executionID, requestID, pendingID, optionID string,
) error {
	b.resolved = append(b.resolved, executionID+"|"+requestID+"|"+pendingID+"|"+optionID)
	return nil
}

func (b *runPermissionBackend) CancelRunPermission(
	_ context.Context, executionID, requestID, pendingID string,
) error {
	b.canceled = append(b.canceled, executionID+"|"+requestID+"|"+pendingID)
	return nil
}

func TestRuntime_RunPermissionDelegatesByExecution(t *testing.T) {
	t.Parallel()
	backend := &runPermissionBackend{fakeBackend: newFakeBackend()}
	runtime := agentruntime.New(backend)

	if err := runtime.ResolveRunPermission(context.Background(), "exec-1", "req-1", "pending-1", "reject"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := runtime.CancelRunPermission(context.Background(), "exec-1", "req-2", "pending-2"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if len(backend.resolved) != 1 || backend.resolved[0] != "exec-1|req-1|pending-1|reject" {
		t.Fatalf("resolved = %v", backend.resolved)
	}
	if len(backend.canceled) != 1 || backend.canceled[0] != "exec-1|req-2|pending-2" {
		t.Fatalf("canceled = %v", backend.canceled)
	}
}

func TestRuntime_RunPermissionRequiresExecutionAndCapability(t *testing.T) {
	t.Parallel()
	withCapability := agentruntime.New(&runPermissionBackend{fakeBackend: newFakeBackend()})
	if err := withCapability.ResolveRunPermission(context.Background(), "", "req", "pending", "opt"); err == nil {
		t.Fatal("resolve without execution ID succeeded")
	}
	without := agentruntime.New(newFakeBackend())
	if err := without.ResolveRunPermission(context.Background(), "exec", "req", "pending", "opt"); !errors.Is(err, agentruntime.ErrUnsupported) {
		t.Fatalf("resolve error = %v, want ErrUnsupported", err)
	}
	if err := without.CancelRunPermission(context.Background(), "exec", "req", "pending"); !errors.Is(err, agentruntime.ErrUnsupported) {
		t.Fatalf("cancel error = %v, want ErrUnsupported", err)
	}
}
