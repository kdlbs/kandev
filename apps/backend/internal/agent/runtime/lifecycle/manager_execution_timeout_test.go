package lifecycle

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCoalescedExecutionAllowsSetupPastOldDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		if constants.AgentLaunchTimeout <= 90*time.Second {
			t.Fatalf("AgentLaunchTimeout = %s, want more than 90s", constants.AgentLaunchTimeout)
		}

		mgr := &Manager{stopCh: make(chan struct{})}
		result := make(chan error, 1)
		go func() {
			_, err := mgr.doCoalescedExecution(context.Background(), "session-setup", func(ctx context.Context) (interface{}, error) {
				select {
				case <-time.After(90 * time.Second):
					return struct{}{}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})
			result <- err
		}()

		if err := <-result; err != nil {
			t.Fatalf("setup crossing old deadline failed: %v", err)
		}
	})
}

func TestTaskHostCreationStartsFreshLaunchPhaseTimeout(t *testing.T) {
	launchErr := errors.New("launch stopped after deadline capture")
	backend := &deadlineCapturingExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		err:          launchErr,
	}
	log := newTestLogger()
	executors := NewExecutorRegistry(log)
	executors.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, executors, &MockCredentialsManager{},
		&MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)

	startedAt := time.Now()
	_, err := mgr.createTaskHostExecution(context.Background(), "task-1", &WorkspaceInfo{
		TaskID: "task-1", TaskEnvironmentID: "env-1", WorkspacePath: "/workspace/task-1",
		ExecutorType: string(models.ExecutorTypeLocal),
	}, true)
	if !errors.Is(err, launchErr) {
		t.Fatalf("task-host creation error = %v, want %v", err, launchErr)
	}
	if !backend.hasDeadline {
		t.Fatal("task-host runtime launch context has no deadline")
	}
	if elapsed := backend.deadline.Sub(startedAt); elapsed < constants.AgentLaunchTimeout-time.Second || elapsed > constants.AgentLaunchTimeout+time.Second {
		t.Fatalf("task-host launch deadline = %v, want %v", elapsed, constants.AgentLaunchTimeout)
	}
}

type deadlineCapturingExecutor struct {
	MockExecutor
	deadline    time.Time
	hasDeadline bool
	err         error
}

func (e *deadlineCapturingExecutor) CreateInstance(
	ctx context.Context,
	_ *ExecutorCreateRequest,
) (*ExecutorInstance, error) {
	e.deadline, e.hasDeadline = ctx.Deadline()
	return nil, e.err
}
