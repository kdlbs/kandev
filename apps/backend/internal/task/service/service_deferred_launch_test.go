package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const deferredLaunchTaskID = "task-deferred"

func TestCreateTaskPersistsDeferredLaunchOriginUserID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-created"})

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-origin", Name: "Origin"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-origin", WorkspaceID: "ws-origin", Name: "flow"}))

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:                 "ws-origin",
		WorkflowID:                  "wf-origin",
		Title:                       "Deferred task",
		StartAgent:                  true,
		RecordAgentProfileRecentUse: true,
		DeferredLaunch: map[string]interface{}{
			"intent":           "start",
			"agent_profile_id": "profile-1",
			"prompt":           "run this later",
		},
	})
	require.NoError(t, err)

	launch, ok := result.Task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "created task must retain its deferred launch intent")
	if got := launch[models.DeferredLaunchUserIDKey]; got != "user-created" {
		t.Fatalf("deferred launch user_id = %v, want user-created", got)
	}
	if got := launch[models.DeferredLaunchRecordRecentUseKey]; got != true {
		t.Fatalf("deferred launch record_recent_use = %v, want true", got)
	}
}

func TestCreateTaskDoesNotPersistDeferredLaunchOriginUserIDWithoutOptIn(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "mcp-user"})

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-mcp", Name: "MCP"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-mcp", WorkspaceID: "ws-mcp", Name: "flow"}))

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-mcp",
		WorkflowID:  "wf-mcp",
		Title:       "MCP deferred task",
		StartAgent:  true,
		DeferredLaunch: map[string]interface{}{
			"intent":           "start",
			"agent_profile_id": "profile-mcp",
			"prompt":           "run this later",
		},
	})
	require.NoError(t, err)

	launch, ok := result.Task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "created task must retain its deferred launch intent")
	if _, exists := launch[models.DeferredLaunchUserIDKey]; exists {
		t.Fatalf("deferred launch user_id = %v, want no recency attribution without opt-in", launch[models.DeferredLaunchUserIDKey])
	}
}

func TestUpdateTaskCannotReplaceDeferredLaunchAttribution(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-protected", Name: "Protected"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-protected", WorkspaceID: "ws-protected", Name: "flow"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-protected", WorkspaceID: "ws-protected", WorkflowID: "wf-protected",
		Title: "Protected deferred task", State: v1.TaskStateCreated,
		Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent":                                "start",
				"agent_profile_id":                      "profile-protected",
				models.DeferredLaunchUserIDKey:          "user-a",
				models.DeferredLaunchRecordRecentUseKey: true,
			},
		},
	}))

	attackerCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "user-b"})
	_, err := svc.UpdateTask(attackerCtx, "task-protected", &UpdateTaskRequest{
		Metadata: map[string]interface{}{
			"ordinary": "allowed",
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent":                                "start",
				"agent_profile_id":                      "profile-protected",
				models.DeferredLaunchUserIDKey:          "user-b",
				models.DeferredLaunchRecordRecentUseKey: true,
			},
		},
	})
	require.NoError(t, err)

	updated, err := repo.GetTask(ctx, "task-protected")
	require.NoError(t, err)
	launch, ok := updated.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "deferred launch attribution must survive generic metadata updates")
	if got := launch[models.DeferredLaunchUserIDKey]; got != "user-a" {
		t.Fatalf("deferred launch user_id = %v, want immutable user-a", got)
	}
	if got := updated.Metadata["ordinary"]; got != "allowed" {
		t.Fatalf("ordinary metadata = %v, want the caller's update preserved", got)
	}

	_, err = svc.UpdateTaskMetadata(attackerCtx, "task-protected", map[string]interface{}{
		"ordinary_from_merge": "allowed",
		models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.DeferredLaunchUserIDKey:          "user-b",
			models.DeferredLaunchRecordRecentUseKey: true,
		},
	})
	require.NoError(t, err)

	updated, err = repo.GetTask(ctx, "task-protected")
	require.NoError(t, err)
	launch, ok = updated.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "deferred launch attribution must survive metadata merges")
	if got := launch[models.DeferredLaunchUserIDKey]; got != "user-a" {
		t.Fatalf("merged deferred launch user_id = %v, want immutable user-a", got)
	}
	if got := updated.Metadata["ordinary_from_merge"]; got != "allowed" {
		t.Fatalf("ordinary metadata from merge = %v, want the caller's update preserved", got)
	}
}

// seedDeferredLaunchTask creates a task carrying a pending start-when-unblocked
// deferred launch, matching what create_task_kandev with blocked_by leaves.
func seedDeferredLaunchTask(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-defer", Name: "Defer"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-defer", WorkspaceID: "ws-defer", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: deferredLaunchTaskID, WorkspaceID: "ws-defer", WorkflowID: "wf-defer",
		Title: "Chain step", State: v1.TaskStateCreated,
		Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent":           "start",
				"agent_profile_id": "profile-1",
				"prompt":           "the brief written before the work started",
				models.DeferredLaunchStartWhenUnblockedKey: true,
			},
		},
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
}

func storedLaunch(t *testing.T, repo *sqliterepo.Repository) map[string]interface{} {
	t.Helper()
	task, err := repo.GetTask(context.Background(), deferredLaunchTaskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	launch, ok := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata = %v, want a deferred launch record", task.Metadata)
	}
	return launch
}

// The bug 4 fix: a pending launch's prompt can be corrected before it fires.
// The rest of the record must survive, or the corrected launch would run
// without its agent profile.
func TestUpdateDeferredLaunchPromptReplacesOnlyThePrompt(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedDeferredLaunchTask(t, repo)

	if _, err := svc.UpdateDeferredLaunchPrompt(
		context.Background(), deferredLaunchTaskID, "what we actually learned"); err != nil {
		t.Fatalf("UpdateDeferredLaunchPrompt: %v", err)
	}

	launch := storedLaunch(t, repo)
	if launch["prompt"] != "what we actually learned" {
		t.Fatalf("prompt = %v, want the replacement", launch["prompt"])
	}
	if launch["agent_profile_id"] != "profile-1" {
		t.Fatalf("agent_profile_id = %v, want the original record preserved", launch["agent_profile_id"])
	}
	if flag, _ := launch[models.DeferredLaunchStartWhenUnblockedKey].(bool); !flag {
		t.Fatal("the start-when-unblocked flag must survive; without it the gate stops recognising the intent")
	}
}

// Once the task has a session it is running, and a launch prompt nothing will
// read must not be silently accepted.
func TestUpdateDeferredLaunchPromptRejectsAStartedTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedDeferredLaunchTask(t, repo)
	ctx := context.Background()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-started", TaskID: deferredLaunchTaskID,
		AgentProfileID: "profile-1", State: models.TaskSessionStateRunning, IsPrimary: true,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	_, err := svc.UpdateDeferredLaunchPrompt(ctx, deferredLaunchTaskID, "too late")
	if !errors.Is(err, ErrDeferredLaunchAlreadyStarted) {
		t.Fatalf("err = %v, want ErrDeferredLaunchAlreadyStarted", err)
	}
	if launch := storedLaunch(t, repo); launch["prompt"] != "the brief written before the work started" {
		t.Fatalf("prompt = %v, want the record left untouched", launch["prompt"])
	}
}

// A cancelled session still means the task was started, so the rejection is not
// filtered by session state.
func TestUpdateDeferredLaunchPromptRejectsATaskWithOnlyACancelledSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedDeferredLaunchTask(t, repo)
	ctx := context.Background()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-cancelled", TaskID: deferredLaunchTaskID,
		AgentProfileID: "profile-1", State: models.TaskSessionStateCancelled, IsPrimary: true,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if _, err := svc.UpdateDeferredLaunchPrompt(ctx, deferredLaunchTaskID, "too late"); !errors.Is(
		err, ErrDeferredLaunchAlreadyStarted) {
		t.Fatalf("err = %v, want ErrDeferredLaunchAlreadyStarted", err)
	}
}

func TestUpdateDeferredLaunchPromptRejectsATaskWithoutOne(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-defer", Name: "Defer"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-defer", WorkspaceID: "ws-defer", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: deferredLaunchTaskID, WorkspaceID: "ws-defer", WorkflowID: "wf-defer",
		Title: "Ordinary", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := svc.UpdateDeferredLaunchPrompt(ctx, deferredLaunchTaskID, "hello"); !errors.Is(
		err, ErrNoPendingDeferredLaunch) {
		t.Fatalf("err = %v, want ErrNoPendingDeferredLaunch", err)
	}
}

// Blanking the prompt would make the launch silently fall back to the task
// description, which is a different operation from correcting a brief.
func TestUpdateDeferredLaunchPromptRejectsABlankPrompt(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedDeferredLaunchTask(t, repo)

	if _, err := svc.UpdateDeferredLaunchPrompt(
		context.Background(), deferredLaunchTaskID, "   \n "); !errors.Is(err, ErrDeferredLaunchPromptEmpty) {
		t.Fatalf("err = %v, want ErrDeferredLaunchPromptEmpty", err)
	}
	if launch := storedLaunch(t, repo); launch["prompt"] != "the brief written before the work started" {
		t.Fatalf("prompt = %v, want the record left untouched", launch["prompt"])
	}
}

// racingSessionRepo fires a concurrent start at the exact point the prompt
// update is vulnerable: after it has decided the task has no session, before it
// writes. Deterministic — the interleaving is the seam, not a sleep.
type racingSessionRepo struct {
	repository.SessionRepository
	tasks *sqliterepo.Repository
	fired bool
}

func (r *racingSessionRepo) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	sessions, err := r.SessionRepository.ListTaskSessions(ctx, taskID)
	if err != nil || r.fired {
		return sessions, err
	}
	r.fired = true
	// The task starts right now: the start path claims the intent atomically,
	// exactly as consumeDeferredLaunchOnStart's claim does.
	if _, removeErr := r.tasks.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyDeferredLaunch); removeErr != nil {
		return nil, removeErr
	}
	return sessions, nil
}

// The resurrection race, reported independently by three reviewers on #2660.
//
// A read-modify-write prompt edit re-creates the metadata key when a start
// consumes it between the read and the write, because UpdateTask persists the
// whole metadata blob it read earlier. The gate then finds a live intent on a
// task that is already running and launches a second session — the exact bug
// this PR exists to fix, reintroduced through the editing path.
//
// The conditional single-key write makes the start win: nothing is written and
// the caller is told the task started.
func TestUpdateDeferredLaunchPromptLosesToAConcurrentStart(t *testing.T) {
	var racing *racingSessionRepo
	svc, _, repo := createTestServiceWithSessionsRepo(t, func(r *sqliterepo.Repository) repository.SessionRepository {
		racing = &racingSessionRepo{SessionRepository: r, tasks: r}
		return racing
	})
	seedDeferredLaunchTask(t, repo)

	_, err := svc.UpdateDeferredLaunchPrompt(context.Background(), deferredLaunchTaskID, "a prompt nothing will read")
	if !errors.Is(err, ErrDeferredLaunchAlreadyStarted) {
		t.Fatalf("err = %v, want ErrDeferredLaunchAlreadyStarted", err)
	}
	if !racing.fired {
		t.Fatal("the fixture never injected the concurrent start; the test proved nothing")
	}

	task, getErr := repo.GetTask(context.Background(), deferredLaunchTaskID)
	if getErr != nil {
		t.Fatalf("GetTask: %v", getErr)
	}
	if _, resurrected := task.Metadata[models.MetaKeyDeferredLaunch]; resurrected {
		t.Fatal("the prompt edit resurrected a launch intent the start had consumed; " +
			"the gate can now gate-launch a second session on a running task")
	}
}
