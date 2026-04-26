package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedTask creates prerequisite workspace + workflow + task rows so that
// foreign-key constraints on task_plans are satisfied.
func seedTask(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-plan", Name: "Plan WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-plan", WorkspaceID: "ws-plan", Name: "WF"})
	now := time.Now().UTC()
	_ = repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-plan",
		WorkflowID:  "wf-plan",
		Title:       "Test",
		State:       v1.TaskStateCreated,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func createTestPlanService(t *testing.T) (*PlanService, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewPlanService(repo, eventBus, log)
	return svc, eventBus, repo
}

func TestPlanService_CreatePlan(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	plan, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-1",
		Title:   "My Plan",
		Content: "Plan content",
	})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}
	if plan.TaskID != "task-1" {
		t.Errorf("expected task_id=task-1, got %s", plan.TaskID)
	}
	if plan.Title != "My Plan" {
		t.Errorf("expected title=My Plan, got %s", plan.Title)
	}
	if plan.Content != "Plan content" {
		t.Errorf("expected content=Plan content, got %s", plan.Content)
	}
	if plan.CreatedBy != "agent" {
		t.Errorf("expected created_by=agent, got %s", plan.CreatedBy)
	}
}

func TestPlanService_CreatePlanUpsert(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	// First create
	plan1, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-1",
		Title:   "Original",
		Content: "v1",
	})
	if err != nil {
		t.Fatalf("first CreatePlan failed: %v", err)
	}

	// Second create with same task_id should upsert, not error
	plan2, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-1",
		Title:   "Updated",
		Content: "v2",
	})
	if err != nil {
		t.Fatalf("second CreatePlan (upsert) failed: %v", err)
	}

	if plan2.ID != plan1.ID {
		t.Errorf("upsert should preserve plan ID: got %s, want %s", plan2.ID, plan1.ID)
	}
	if plan2.Title != "Updated" {
		t.Errorf("expected title=Updated, got %s", plan2.Title)
	}
	if plan2.Content != "v2" {
		t.Errorf("expected content=v2, got %s", plan2.Content)
	}
}

func TestPlanService_CreatePlanRequiresTaskID(t *testing.T) {
	svc, _, _ := createTestPlanService(t)
	ctx := context.Background()

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{Content: "x"})
	if err != ErrTaskIDRequired {
		t.Errorf("expected ErrTaskIDRequired, got %v", err)
	}
}

func TestPlanService_GetPlan(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	// Non-existent returns nil, nil
	plan, err := svc.GetPlan(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}
	if plan != nil {
		t.Errorf("expected nil for task with no plan, got %+v", plan)
	}

	// Create then get
	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: "c"})
	plan, err = svc.GetPlan(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}
	if plan == nil || plan.Content != "c" {
		t.Errorf("expected plan with content=c, got %+v", plan)
	}
}

func TestPlanService_UpdatePlan(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Title: "T1", Content: "c1"})

	updated, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-1", Content: "c2"})
	if err != nil {
		t.Fatalf("UpdatePlan failed: %v", err)
	}
	if updated.Content != "c2" {
		t.Errorf("expected content=c2, got %s", updated.Content)
	}
	// Title preserved when empty
	if updated.Title != "T1" {
		t.Errorf("expected title=T1 (preserved), got %s", updated.Title)
	}
}

func TestPlanService_UpdatePlanNotFound(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-1", Content: "x"})
	if err != ErrTaskPlanNotFound {
		t.Errorf("expected ErrTaskPlanNotFound, got %v", err)
	}
}

func TestPlanService_DeletePlan(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: "c"})
	if err := svc.DeletePlan(ctx, "task-1"); err != nil {
		t.Fatalf("DeletePlan failed: %v", err)
	}

	plan, _ := svc.GetPlan(ctx, "task-1")
	if plan != nil {
		t.Errorf("expected nil after delete, got %+v", plan)
	}
}

func TestPlanService_DeletePlanNotFound(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	err := svc.DeletePlan(ctx, "task-1")
	if err != ErrTaskPlanNotFound {
		t.Errorf("expected ErrTaskPlanNotFound, got %v", err)
	}
}

func TestPlanService_CreatesInitialRevision(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rev")

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-rev", Content: "v1",
		AuthorKind: "agent", AuthorName: "Claude",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	list, _ := svc.ListRevisions(ctx, "task-rev")
	if len(list) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(list))
	}
	if list[0].AuthorName != "Claude" || list[0].AuthorKind != "agent" {
		t.Errorf("unexpected author: %+v", list[0])
	}
	if list[0].RevisionNumber != 1 {
		t.Errorf("expected rev #1, got %d", list[0].RevisionNumber)
	}
}

func TestPlanService_CoalescesWithinWindow(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-co")
	svc.coalesceWindow = 10 * time.Minute // force generous window

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-co", Content: "v1",
		AuthorKind: "agent", AuthorName: "Claude",
	})
	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-co", Content: "v2",
		AuthorKind: "agent", AuthorName: "Claude",
	})

	list, _ := svc.ListRevisions(ctx, "task-co")
	if len(list) != 1 {
		t.Fatalf("expected coalesced to 1, got %d", len(list))
	}
	if list[0].Content != "v2" {
		t.Errorf("expected merged content v2, got %q", list[0].Content)
	}
}

func TestPlanService_AppendsWhenWindowExpired(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-win")
	svc.coalesceWindow = 0 // disable coalescing

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-win", Content: "v1",
		AuthorKind: "agent", AuthorName: "Claude",
	})
	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-win", Content: "v2",
		AuthorKind: "agent", AuthorName: "Claude",
	})

	list, _ := svc.ListRevisions(ctx, "task-win")
	if len(list) != 2 {
		t.Fatalf("expected 2 separate revisions, got %d", len(list))
	}
}

func TestPlanService_AuthorSwitchBreaksCoalesce(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-sw")
	svc.coalesceWindow = 10 * time.Minute

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-sw", Content: "agent-wrote",
		AuthorKind: "agent", AuthorName: "Claude",
	})
	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-sw", Content: "user-edited",
		AuthorKind: "user", AuthorName: "Alice",
	})

	list, _ := svc.ListRevisions(ctx, "task-sw")
	if len(list) != 2 {
		t.Fatalf("expected 2 revisions (author switch breaks coalesce), got %d", len(list))
	}
	if list[0].AuthorKind != "user" || list[1].AuthorKind != "agent" {
		t.Errorf("unexpected order: [%s, %s]", list[0].AuthorKind, list[1].AuthorKind)
	}
}

func TestPlanService_RevertToEarlierRevision(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rv")
	svc.coalesceWindow = 0

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-rv", Content: "v1", AuthorKind: "agent", AuthorName: "Claude"})
	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-rv", Content: "v2", AuthorKind: "agent", AuthorName: "Claude"})
	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-rv", Content: "v3", AuthorKind: "agent", AuthorName: "Claude"})

	list, _ := svc.ListRevisions(ctx, "task-rv")
	if len(list) != 3 {
		t.Fatalf("expected 3 before revert, got %d", len(list))
	}
	v1 := list[2]

	revert, err := svc.RevertPlan(ctx, RevertPlanRequest{
		TaskID: "task-rv", TargetRevisionID: v1.ID, AuthorName: "Alice",
	})
	if err != nil {
		t.Fatalf("revert: %v", err)
	}
	if revert.RevertOfRevisionID == nil || *revert.RevertOfRevisionID != v1.ID {
		t.Errorf("revert_of_revision_id mismatch: %v", revert.RevertOfRevisionID)
	}
	if revert.AuthorKind != "user" || revert.AuthorName != "Alice" {
		t.Errorf("expected user/Alice, got %s/%s", revert.AuthorKind, revert.AuthorName)
	}
	if revert.RevisionNumber != 4 {
		t.Errorf("expected rev #4, got %d", revert.RevisionNumber)
	}

	head, _ := svc.GetPlan(ctx, "task-rv")
	if head.Content != "v1" {
		t.Errorf("expected HEAD content v1, got %q", head.Content)
	}
}

func TestPlanService_RevertNeverCoalesces(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rvc")
	svc.coalesceWindow = 10 * time.Minute

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-rvc", Content: "v1", AuthorKind: "agent", AuthorName: "Claude"})
	list, _ := svc.ListRevisions(ctx, "task-rvc")
	v1 := list[0]

	// Two reverts by the same user in quick succession must remain separate rows.
	_, _ = svc.RevertPlan(ctx, RevertPlanRequest{TaskID: "task-rvc", TargetRevisionID: v1.ID, AuthorName: "Alice"})
	_, _ = svc.RevertPlan(ctx, RevertPlanRequest{TaskID: "task-rvc", TargetRevisionID: v1.ID, AuthorName: "Alice"})

	list, _ = svc.ListRevisions(ctx, "task-rvc")
	if len(list) != 3 {
		t.Fatalf("expected 3 revisions (1 original + 2 reverts), got %d", len(list))
	}
}

func TestPlanService_RevertRejectsWrongTask(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-x")
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-y", WorkspaceID: "ws-plan", WorkflowID: "wf-plan", Title: "Y",
		State: v1.TaskStateCreated, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-x", Content: "x", AuthorKind: "agent"})
	xList, _ := svc.ListRevisions(ctx, "task-x")
	xRev := xList[0]

	_, err := svc.RevertPlan(ctx, RevertPlanRequest{
		TaskID: "task-y", TargetRevisionID: xRev.ID, AuthorName: "Alice",
	})
	if err != ErrRevisionTaskMismatch {
		t.Errorf("expected ErrRevisionTaskMismatch, got %v", err)
	}
}

func TestPlanService_AgentAuthorNameFromSession(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-an")

	// Seed an active session with an agent profile snapshot. The MCP path
	// resolves the agent's display name from this snapshot when the request
	// doesn't carry an explicit author_name.
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:               "sess-an",
		TaskID:           "task-an",
		AgentExecutionID: "exec-an",
		AgentProfileID:   "ap-claude",
		AgentProfileSnapshot: map[string]interface{}{
			"id":         "ap-claude",
			"name":       "Claude Sonnet 4.5",
			"agent_id":   "claude",
			"agent_name": "claude",
		},
		State:     models.TaskSessionState("RUNNING"),
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession failed: %v", err)
	}

	// MCP path: created_by=agent, no author_name provided.
	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:    "task-an",
		Content:   "first draft",
		CreatedBy: "agent",
	})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}

	list, _ := svc.ListRevisions(ctx, "task-an")
	if len(list) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(list))
	}
	if list[0].AuthorKind != "agent" {
		t.Errorf("expected author_kind=agent, got %q", list[0].AuthorKind)
	}
	if list[0].AuthorName != "Claude Sonnet 4.5" {
		t.Errorf("expected author_name resolved from session snapshot, got %q", list[0].AuthorName)
	}
}

func TestPlanService_AgentAuthorNameFallsBackWhenNoSession(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-an2")

	// No session seeded → resolution returns "" → resolveAuthor falls back to "Agent".
	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:    "task-an2",
		Content:   "first draft",
		CreatedBy: "agent",
	})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}

	list, _ := svc.ListRevisions(ctx, "task-an2")
	if list[0].AuthorName != defaultAgentAuthorFallback {
		t.Errorf("expected fallback %q, got %q", defaultAgentAuthorFallback, list[0].AuthorName)
	}
}

func TestPlanService_RevertMissingRevision(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-mr")

	_, err := svc.RevertPlan(ctx, RevertPlanRequest{
		TaskID: "task-mr", TargetRevisionID: "does-not-exist", AuthorName: "Alice",
	})
	if err != ErrRevisionNotFound {
		t.Errorf("expected ErrRevisionNotFound, got %v", err)
	}
}
