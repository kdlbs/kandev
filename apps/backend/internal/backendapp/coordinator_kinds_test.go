package backendapp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type fakeKindTaskAPI struct {
	task       *taskmodels.Task
	taskErr    error
	session    *taskmodels.TaskSession
	sessionErr error
	record     *taskmodels.ExecutorRunning
	recordErr  error
}

func (f *fakeKindTaskAPI) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return f.task, f.taskErr
}

func (f *fakeKindTaskAPI) GetPrimarySession(context.Context, string) (*taskmodels.TaskSession, error) {
	return f.session, f.sessionErr
}

func (f *fakeKindTaskAPI) GetExecutorRunningBySessionID(context.Context, string) (*taskmodels.ExecutorRunning, error) {
	return f.record, f.recordErr
}

func newKindReader(api *fakeKindTaskAPI) *coordinatorKindReader {
	return &coordinatorKindReader{tasks: api, liveExec: func(id string) bool { return id == "live" }}
}

func TestCoordinatorKindReader_GetTarget(t *testing.T) {
	base := func() *fakeKindTaskAPI {
		return &fakeKindTaskAPI{
			task:    &taskmodels.Task{ID: "t", WorkspaceID: "ws", WorkflowID: "wf", WorkflowStepID: "s"},
			session: &taskmodels.TaskSession{ID: "sess", State: taskmodels.TaskSessionStateWaitingForInput},
			record:  &taskmodels.ExecutorRunning{},
		}
	}
	t.Run("maps placement, session state and executor record", func(t *testing.T) {
		got, err := newKindReader(base()).GetTarget(context.Background(), "t")
		if err != nil || got.WorkspaceID != "ws" || got.Primary == nil || got.Primary.State != "WAITING_FOR_INPUT" || !got.Primary.HasExecutorRecord {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("missing task maps to the seam sentinel", func(t *testing.T) {
		api := base()
		api.taskErr = fmt.Errorf("wrapped: %w", repoerrors.ErrTaskNotFound)
		if _, err := newKindReader(api).GetTarget(context.Background(), "t"); !errors.Is(err, coordinator.ErrTaskNotFound) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no primary session leaves Primary nil", func(t *testing.T) {
		api := base()
		api.sessionErr = repoerrors.ErrNoPrimarySession
		got, err := newKindReader(api).GetTarget(context.Background(), "t")
		if err != nil || got.Primary != nil {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("missing executor record is reported false, other errors fail", func(t *testing.T) {
		api := base()
		api.record, api.recordErr = nil, fmt.Errorf("x: %w", taskmodels.ErrExecutorRunningNotFound)
		got, err := newKindReader(api).GetTarget(context.Background(), "t")
		if err != nil || got.Primary.HasExecutorRecord {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		api.recordErr = errors.New("db down")
		if _, err := newKindReader(api).GetTarget(context.Background(), "t"); err == nil {
			t.Fatal("a read failure must not be reported as no record")
		}
	})
	t.Run("live execution", func(t *testing.T) {
		r := newKindReader(base())
		if !r.HasLiveExecution(context.Background(), "live") || r.HasLiveExecution(context.Background(), "other") {
			t.Fatal("HasLiveExecution must delegate to the runtime")
		}
	})
}

func TestCoordinatorResumer_NilExecutionMeansDeferred(t *testing.T) {
	r := &coordinatorResumer{resume: func(context.Context, string, string) (*executor.TaskExecution, error) { return nil, nil }}
	if started, err := r.ResumeTaskSession(context.Background(), "t", "s"); err != nil || started {
		t.Fatalf("started=%v err=%v, want deferred", started, err)
	}
	r.resume = func(context.Context, string, string) (*executor.TaskExecution, error) {
		return &executor.TaskExecution{}, nil
	}
	if started, err := r.ResumeTaskSession(context.Background(), "t", "s"); err != nil || !started {
		t.Fatalf("started=%v err=%v, want started", started, err)
	}
}
