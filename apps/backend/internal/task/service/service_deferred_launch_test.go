package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const deferredLaunchTaskID = "task-deferred"

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
