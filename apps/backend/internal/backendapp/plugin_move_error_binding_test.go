package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// pluginMoveErrorBindingHarness wires the same two repositories and services
// production boots (internal/task's own workflow table for
// validateTaskMove's GetWorkflow/workspace checks, plus the separate
// internal/workflow package for step lookups via workflowStepGetterAdapter),
// so the error text classifyPluginMoveError matches against is whatever the
// real validators produce today, not a hand-written stand-in.
type pluginMoveErrorBindingHarness struct {
	adapter pluginsTaskWriterAdapter
	repo    *sqliterepo.Repository
}

func newPluginMoveErrorBindingHarness(t *testing.T) *pluginMoveErrorBindingHarness {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "plugin-move-error-binding.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	taskRepo, cleanup, err := repository.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	workflowRepo, err := workflowrepository.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("workflow repository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:       taskRepo,
		Tasks:            taskRepo,
		TaskRepos:        taskRepo,
		Workflows:        taskRepo,
		Messages:         taskRepo,
		Turns:            taskRepo,
		Sessions:         taskRepo,
		GitSnapshots:     taskRepo,
		RepoEntities:     taskRepo,
		Executors:        taskRepo,
		Environments:     taskRepo,
		TaskEnvironments: taskRepo,
		Reviews:          taskRepo,
		ResourceCleanups: taskRepo,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})
	workflowSvc := workflowservice.NewService(workflowRepo, log)
	taskSvc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: workflowSvc})

	ctx := context.Background()
	require.NoError(t, taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-1", Name: "Workspace 1"}))
	require.NoError(t, taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-2", Name: "Workspace 2"}))
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf-home", WorkspaceID: "ws-1", Name: "Home"}))
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf-target", WorkspaceID: "ws-1", Name: "Target"}))
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf-other-workspace", WorkspaceID: "ws-2", Name: "Other"}))
	require.NoError(t, workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0}))
	require.NoError(t, workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "step-home", WorkflowID: "wf-home", Name: "Home", Position: 0}))

	return &pluginMoveErrorBindingHarness{
		adapter: pluginsTaskWriterAdapter{svc: taskSvc},
		repo:    taskRepo,
	}
}

func (h *pluginMoveErrorBindingHarness) createTask(t *testing.T, id string, archived bool) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, h.repo.CreateTask(ctx, &taskmodels.Task{
		ID:             id,
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-home",
		WorkflowStepID: "step-home",
		Title:          id,
		State:          v1.TaskStateTODO,
	}))
	if archived {
		require.NoError(t, h.repo.ArchiveTask(ctx, id))
	}
}

// TestClassifyPluginMoveError_BindsToRealValidatorStrings pins AC-004.6: the
// binding error-mapping table in classifyPluginMoveError must be exercised
// against errors the real validateTaskMove/GetWorkflow/GetStep code paths
// actually produce, not hand-written stand-in strings — so a validator
// wording change that breaks the classifier's substring match fails this
// test, per Review round 1's must-fix finding (test-supervisor proved by
// mutation that TestClassifyPluginMoveError's stand-in literals do not
// notice such a change).
func TestClassifyPluginMoveError_BindsToRealValidatorStrings(t *testing.T) {
	wf := "wf-target"

	t.Run("archived task, AC-001.7", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-archived", true)
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-archived", WorkflowStepID: "step-target", WorkflowID: &wf, Source: "plugin:acme",
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
	})

	t.Run("active session, AC-001.8", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-live-session", false)
		require.NoError(t, h.repo.CreateTaskSession(context.Background(), &taskmodels.TaskSession{
			ID: "session-live", TaskID: "task-live-session", State: taskmodels.TaskSessionStateRunning, IsPrimary: true,
		}))
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-live-session", WorkflowStepID: "step-target", WorkflowID: &wf, Source: "plugin:acme",
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
	})

	t.Run("unknown workflow, AC-001.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-unknown-wf", false)
		missing := "wf-does-not-exist"
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-unknown-wf", WorkflowStepID: "step-target", WorkflowID: &missing, Source: "plugin:acme",
		})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("different workspace, AC-001.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-cross-workspace", false)
		other := "wf-other-workspace"
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-cross-workspace", WorkflowStepID: "step-target", WorkflowID: &other, Source: "plugin:acme",
		})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("unknown step, AC-001.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-unknown-step", false)
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-unknown-step", WorkflowStepID: "step-does-not-exist", WorkflowID: &wf, Source: "plugin:acme",
		})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("step not in workflow, AC-001.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-step-wrong-workflow", false)
		// step-home is a real step, but it belongs to wf-home, not wf-target.
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-step-wrong-workflow", WorkflowStepID: "step-home", WorkflowID: &wf, Source: "plugin:acme",
		})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("task not found, AC-005.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-missing", WorkflowStepID: "step-target", WorkflowID: &wf, Source: "plugin:acme",
		})
		require.Equal(t, codes.NotFound, status.Code(err))
	})
}

// TestResolvePluginMoveWorkflowID_StaleReadFailsSafeNotSilentlyWrong pins
// TEST-003 (Review round 1, LOW/non-blocking, converges with Spec Review
// round 5's Q-001): an omitted WorkflowID is resolved by a separate GetTask
// pre-read (resolvePluginMoveWorkflowID) rather than atomically with the move
// itself. If the task's workflow changes between that pre-read and the move
// commit — the accepted race — the stale workflow id must never let the move
// silently land somewhere it shouldn't; validateTaskMove's
// targetStep.WorkflowID != workflowID check must reject it as
// InvalidArgument instead.
func TestResolvePluginMoveWorkflowID_StaleReadFailsSafeNotSilentlyWrong(t *testing.T) {
	h := newPluginMoveErrorBindingHarness(t)
	h.createTask(t, "task-race", false)

	staleWorkflowID, err := h.adapter.resolvePluginMoveWorkflowID(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-race",
	})
	require.NoError(t, err)
	require.Equal(t, "wf-home", staleWorkflowID, "pre-read should observe the task's workflow at that moment")

	// Simulate the race: another caller moves the task to a different
	// workflow before the plugin's own move (using the now-stale pre-read)
	// commits.
	_, err = h.adapter.svc.MoveTaskWithOptions(context.Background(), "task-race", "wf-target", "step-target", 0, taskservice.MoveTaskOptions{})
	require.NoError(t, err)

	_, err = h.adapter.svc.MoveTaskWithOptions(context.Background(), "task-race", staleWorkflowID, "step-target", 0, taskservice.MoveTaskOptions{})
	require.Equal(t, codes.InvalidArgument, status.Code(classifyPluginMoveError(err)), "a stale workflow id must fail safe, never silently move the task")

	final, getErr := h.repo.GetTask(context.Background(), "task-race")
	require.NoError(t, getErr)
	require.Equal(t, "step-target", final.WorkflowStepID, "the failed stale move must not have changed the step set by the interceding real move")
	require.Equal(t, "wf-target", final.WorkflowID)
}
