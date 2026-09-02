package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedTask creates prerequisite workspace + workflow + task rows so that
// foreign-key constraints on task_plans are satisfied.
//
// Priority is set to "medium" because the office priority migration
// (when applied alongside task migrations) adds a CHECK constraint
// against the canonical four-value enum on tasks.priority. Service-level
// CreateTask defaults empty values to "medium"; this seed helper writes
// to the repo directly so it must set the value explicitly.
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
		Priority:    "medium",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func seedWorkflowStep(t *testing.T, repo *sqliterepo.Repository, taskID, stepID, name, color string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.DB().ExecContext(context.Background(), `
		INSERT INTO workflow_steps (id, workflow_id, name, position, color, created_at, updated_at)
		VALUES (?, 'wf-plan', ?, 0, ?, ?, ?)
	`, stepID, name, color, now, now); err != nil {
		t.Fatalf("insert workflow step %s: %v", stepID, err)
	}
	if _, err := repo.DB().ExecContext(context.Background(), `
		UPDATE tasks SET workflow_step_id = ? WHERE id = ?
	`, stepID, taskID); err != nil {
		t.Fatalf("set task workflow step %s: %v", stepID, err)
	}
}

func seedSession(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID, sessionID string) {
	t.Helper()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
}

func createTestPlanService(t *testing.T) (*PlanService, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewPlanService(repo, eventBus, log)
	return svc, eventBus, repo
}

func TestResolveCoalesceWindowUsesTypedStartupValue(t *testing.T) {
	if got := resolveCoalesceWindow(2400 * time.Millisecond); got != 2400*time.Millisecond {
		t.Fatalf("coalesce window = %s, want 2.4s", got)
	}
}

type nilMarkPlanRepo struct {
	*sqliterepo.Repository
}

func (r *nilMarkPlanRepo) MarkTaskPlanImplementationStarted(ctx context.Context, taskID, sessionID, actor string) (*models.TaskPlan, error) {
	return nil, nil
}

func TestPlanService_CreatePlan(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	result, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-1",
		Title:   "My Plan",
		Content: "Plan content",
	})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}
	plan := result.Plan
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
	result1, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-1",
		Title:   "Original",
		Content: "v1",
	})
	if err != nil {
		t.Fatalf("first CreatePlan failed: %v", err)
	}

	// Second create with same task_id should upsert, not error
	result2, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-1",
		Title:   "Updated",
		Content: "v2",
	})
	if err != nil {
		t.Fatalf("second CreatePlan (upsert) failed: %v", err)
	}
	plan1, plan2 := result1.Plan, result2.Plan

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

	updateResult, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-1", Content: "c2"})
	if err != nil {
		t.Fatalf("UpdatePlan failed: %v", err)
	}
	updated := updateResult.Plan
	if updated.Content != "c2" {
		t.Errorf("expected content=c2, got %s", updated.Content)
	}
	// Title preserved when empty
	if updated.Title != "T1" {
		t.Errorf("expected title=T1 (preserved), got %s", updated.Title)
	}
}

func TestPlanService_MarkImplementationStartedIsDurableAndIdempotent(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-impl")
	seedSession(t, ctx, repo, "task-impl", "session-1")
	seedSession(t, ctx, repo, "task-impl", "session-2")

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:    "task-impl",
		Title:     "Plan",
		Content:   "Ship the toolbar",
		CreatedBy: "user",
	})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}

	marked, err := svc.MarkImplementationStarted(ctx, MarkImplementationStartedRequest{
		TaskID:    "task-impl",
		SessionID: "session-1",
		Actor:     "user",
	})
	if err != nil {
		t.Fatalf("MarkImplementationStarted failed: %v", err)
	}
	if marked.ImplementationStartedAt == nil {
		t.Fatal("expected implementation_started_at to be set")
	}
	if marked.ImplementationStartedSessionID == nil || *marked.ImplementationStartedSessionID != "session-1" {
		t.Fatalf("expected session marker session-1, got %v", marked.ImplementationStartedSessionID)
	}
	if marked.ImplementationStartedBy == nil || *marked.ImplementationStartedBy != "user" {
		t.Fatalf("expected actor marker user, got %v", marked.ImplementationStartedBy)
	}

	firstStartedAt := *marked.ImplementationStartedAt
	idempotent, err := svc.MarkImplementationStarted(ctx, MarkImplementationStartedRequest{
		TaskID:    "task-impl",
		SessionID: "session-2",
		Actor:     "agent",
	})
	if err != nil {
		t.Fatalf("second MarkImplementationStarted failed: %v", err)
	}
	if !idempotent.ImplementationStartedAt.Equal(firstStartedAt) {
		t.Fatalf("expected started_at to remain %s, got %s", firstStartedAt, *idempotent.ImplementationStartedAt)
	}
	if idempotent.ImplementationStartedSessionID == nil || *idempotent.ImplementationStartedSessionID != "session-1" {
		t.Fatalf("expected idempotent session marker session-1, got %v", idempotent.ImplementationStartedSessionID)
	}
	if idempotent.ImplementationStartedBy == nil || *idempotent.ImplementationStartedBy != "user" {
		t.Fatalf("expected idempotent actor marker user, got %v", idempotent.ImplementationStartedBy)
	}

	updateResult, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID:    "task-impl",
		Content:   "Ship the toolbar after review",
		CreatedBy: "user",
	})
	if err != nil {
		t.Fatalf("UpdatePlan failed: %v", err)
	}
	updated := updateResult.Plan
	if updated.ImplementationStartedAt == nil || !updated.ImplementationStartedAt.Equal(firstStartedAt) {
		t.Fatalf("expected update to preserve implementation marker, got %v", updated.ImplementationStartedAt)
	}
}

func TestPlanService_MarkImplementationStartedRejectsCrossTaskSession(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-impl")
	seedTask(t, ctx, repo, "task-other")
	seedSession(t, ctx, repo, "task-other", "session-other")

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-impl",
		Title:   "Plan",
		Content: "Ship the toolbar",
	})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}

	_, err = svc.MarkImplementationStarted(ctx, MarkImplementationStartedRequest{
		TaskID:    "task-impl",
		SessionID: "session-other",
		Actor:     "user",
	})
	if err != ErrSessionTaskMismatch {
		t.Fatalf("expected ErrSessionTaskMismatch, got %v", err)
	}
}

func TestPlanService_MarkImplementationStartedRejectsMissingSession(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-impl")

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-impl",
		Title:   "Plan",
		Content: "Ship the toolbar",
	})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}

	_, err = svc.MarkImplementationStarted(ctx, MarkImplementationStartedRequest{
		TaskID:    "task-impl",
		SessionID: "missing-session",
		Actor:     "user",
	})
	if err != ErrSessionTaskMismatch {
		t.Fatalf("expected ErrSessionTaskMismatch, got %v", err)
	}
}

func TestPlanService_MarkImplementationStartedHandlesNilPlanAfterWrite(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewPlanService(&nilMarkPlanRepo{Repository: repo}, eventBus, log)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-impl")
	seedSession(t, ctx, repo, "task-impl", "session-1")

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  "task-impl",
		Title:   "Plan",
		Content: "Ship the toolbar",
	})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}
	eventBus.ClearEvents()

	_, err = svc.MarkImplementationStarted(ctx, MarkImplementationStartedRequest{
		TaskID:    "task-impl",
		SessionID: "session-1",
		Actor:     "user",
	})
	if err != ErrTaskPlanNotFound {
		t.Fatalf("expected ErrTaskPlanNotFound, got %v", err)
	}
	if got := len(eventBus.GetPublishedEvents()); got != 0 {
		t.Fatalf("expected no events after nil plan marker, got %d", got)
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

// TestPlanService_GetLatestRevision pins the cheap latest-revision accessor:
// it must report the same revision number ListRevisions()[0] would, without
// requiring callers to load every revision's content just to read one int.
func TestPlanService_GetLatestRevision(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-latest")
	svc.coalesceWindow = 0 // disable coalescing so each write is a new revision

	// No plan yet: nil, nil.
	latest, err := svc.GetLatestRevision(ctx, "task-latest")
	if err != nil {
		t.Fatalf("GetLatestRevision (no plan): %v", err)
	}
	if latest != nil {
		t.Errorf("expected nil latest revision before any plan exists, got %+v", latest)
	}

	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-latest", Content: "v1",
		AuthorKind: "agent", AuthorName: "Claude",
	})
	_, _ = svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-latest", Content: "v2",
		AuthorKind: "agent", AuthorName: "Claude",
	})

	latest, err = svc.GetLatestRevision(ctx, "task-latest")
	if err != nil {
		t.Fatalf("GetLatestRevision: %v", err)
	}
	if latest == nil {
		t.Fatal("expected a latest revision after two writes, got nil")
	}
	if latest.RevisionNumber != 2 || latest.Content != "v2" {
		t.Errorf("latest = %+v, want revision 2 with content v2", latest)
	}

	list, _ := svc.ListRevisions(ctx, "task-latest")
	if len(list) == 0 {
		t.Fatal("GetLatestRevision: ListRevisions returned no revisions after two writes")
	}
	if list[0].RevisionNumber != latest.RevisionNumber {
		t.Errorf("GetLatestRevision disagrees with ListRevisions()[0]: got %d, want %d",
			latest.RevisionNumber, list[0].RevisionNumber)
	}
}

func TestPlanService_GetLatestRevisionRequiresTaskID(t *testing.T) {
	svc, _, _ := createTestPlanService(t)
	ctx := context.Background()

	_, err := svc.GetLatestRevision(ctx, "")
	if err != ErrTaskIDRequired {
		t.Errorf("expected ErrTaskIDRequired, got %v", err)
	}
}

func TestPlanService_ForceNewRevisionPreventsCoalesce(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-force")
	svc.coalesceWindow = 10 * time.Minute // same generous window as the coalesce test

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-force", Content: "v1",
		AuthorKind: "agent", AuthorName: "Claude",
	})
	if err != nil {
		t.Fatalf("initial create: %v", err)
	}
	// Same author, same window — would coalesce by default (see
	// TestPlanService_CoalescesWithinWindow) but must not when a truncating
	// write forces a new revision so the pre-write content survives.
	_, err = svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-force", Content: "v2",
		AuthorKind: "agent", AuthorName: "Claude",
		ForceNewRevision: true,
	})
	if err != nil {
		t.Fatalf("forced update: %v", err)
	}

	list, err := svc.ListRevisions(ctx, "task-force")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 separate revisions (forced), got %d", len(list))
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
		State: v1.TaskStateCreated, Priority: "medium",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
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

// fakePlanWorkflowStepGetter is a minimal PlanWorkflowStepGetter test double
// keyed by step ID.
type fakePlanWorkflowStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (f *fakePlanWorkflowStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	return f.steps[stepID], nil
}

func TestPlanService_StampsWorkflowStepAtWriteTime(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-step")
	seedWorkflowStep(t, repo, "task-step", "step-build", "Build", "bg-blue-500")
	svc.SetWorkflowStepGetter(&fakePlanWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-build": {ID: "step-build", Name: "Build", Color: "bg-blue-500"},
	}})

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-step", Content: "v1", AuthorKind: "agent", AuthorName: "Claude",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	list, _ := svc.ListRevisions(ctx, "task-step")
	if len(list) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(list))
	}
	rev := list[0]
	if rev.WorkflowStepID != "step-build" || rev.WorkflowStepName != "Build" || rev.WorkflowStepColor != "bg-blue-500" {
		t.Errorf("expected workflow step stamp, got %+v", rev)
	}
}

// TestPlanService_NilWorkflowStepGetterIsSafe pins the "nil getter must be
// safe" requirement: a plan service without SetWorkflowStepGetter wired
// (the common case for callers that don't need this metadata) must still
// write revisions successfully, with the step fields left empty.
func TestPlanService_NilWorkflowStepGetterIsSafe(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-nogetter")

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-nogetter", Content: "v1", AuthorKind: "agent", AuthorName: "Claude",
	})
	if err != nil {
		t.Fatalf("create with nil getter: %v", err)
	}

	list, _ := svc.ListRevisions(ctx, "task-nogetter")
	if len(list) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(list))
	}
	if list[0].WorkflowStepID != "" || list[0].WorkflowStepName != "" || list[0].WorkflowStepColor != "" {
		t.Errorf("expected empty workflow step fields with no getter wired, got %+v", list[0])
	}
}

// TestPlanService_CoalescePreservesOriginalWorkflowStep pins the "coalesce
// attributes to the original write" rule for the new stamp columns, matching
// the pre-existing author+number preservation: a coalesced write must not
// re-stamp the row with whatever step the task has moved to since.
func TestPlanService_CoalescePreservesOriginalWorkflowStep(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-co-step")
	svc.coalesceWindow = 10 * time.Minute
	getter := &fakePlanWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-build":  {ID: "step-build", Name: "Build", Color: "bg-blue-500"},
		"step-review": {ID: "step-review", Name: "Review", Color: "bg-purple-500"},
	}}
	svc.SetWorkflowStepGetter(getter)
	seedWorkflowStep(t, repo, "task-co-step", "step-build", "Build", "bg-blue-500")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-co-step", Content: "v1", AuthorKind: "agent", AuthorName: "Claude",
	}); err != nil {
		t.Fatalf("create v1: %v", err)
	}

	// Task moves to a new step before the coalescing second write arrives.
	seedWorkflowStep(t, repo, "task-co-step", "step-review", "Review", "bg-purple-500")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-co-step", Content: "v2", AuthorKind: "agent", AuthorName: "Claude",
	}); err != nil {
		t.Fatalf("create v2 (coalesced): %v", err)
	}

	list, _ := svc.ListRevisions(ctx, "task-co-step")
	if len(list) != 1 {
		t.Fatalf("expected coalesced to 1 revision, got %d", len(list))
	}
	if list[0].WorkflowStepID != "step-build" || list[0].WorkflowStepName != "Build" {
		t.Errorf("expected coalesced revision to keep original step-build stamp, got %+v", list[0])
	}
}

// TestPlanService_RevisionEventCarriesContentLengthAndWorkflowStep pins the
// live WebSocket path to the same metadata the HTTP list/get paths already
// expose: a client with the task panel open, relying solely on
// task_plan.revision.created pushes, must still see content_length and the
// workflow step stamp without a refetch.
func TestPlanService_RevisionEventCarriesContentLengthAndWorkflowStep(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-ws-meta")
	seedWorkflowStep(t, repo, "task-ws-meta", "step-build", "Build", "bg-blue-500")
	svc.SetWorkflowStepGetter(&fakePlanWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-build": {ID: "step-build", Name: "Build", Color: "bg-blue-500"},
	}})

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-ws-meta", Content: "hello", AuthorKind: "agent", AuthorName: "Claude",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	var payload map[string]interface{}
	for _, evt := range eventBus.GetPublishedEvents() {
		if evt.Type == events.TaskPlanRevisionCreated {
			payload, _ = evt.Data.(map[string]interface{})
		}
	}
	if payload == nil {
		t.Fatalf("expected a %s event, got %#v", events.TaskPlanRevisionCreated, eventBus.GetPublishedEvents())
	}
	if payload["content_length"] != 5 {
		t.Errorf("expected content_length 5 (len of \"hello\"), got %#v", payload["content_length"])
	}
	if payload["workflow_step_id"] != "step-build" {
		t.Errorf("expected workflow_step_id step-build, got %#v", payload["workflow_step_id"])
	}
	if payload["workflow_step_name"] != "Build" {
		t.Errorf("expected workflow_step_name Build, got %#v", payload["workflow_step_name"])
	}
	if payload["workflow_step_color"] != "bg-blue-500" {
		t.Errorf("expected workflow_step_color bg-blue-500, got %#v", payload["workflow_step_color"])
	}
}

// TestPlanService_RevertPlanStampsWorkflowStep pins the fix for RevertPlan
// bypassing upsertPlan's currentWorkflowStepStamp call: a revert performed
// while the task sits on a workflow step must produce a revision row with
// that step's badge, not an empty one, matching its non-revert neighbours.
func TestPlanService_RevertPlanStampsWorkflowStep(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-revert-step")
	svc.SetWorkflowStepGetter(&fakePlanWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-review": {ID: "step-review", Name: "Review", Color: "bg-purple-500"},
	}})

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-revert-step", Content: "v1", AuthorKind: "agent", AuthorName: "Claude",
	})
	if err != nil {
		t.Fatalf("create v1: %v", err)
	}
	list, _ := svc.ListRevisions(ctx, "task-revert-step")
	v1 := list[0]

	seedWorkflowStep(t, repo, "task-revert-step", "step-review", "Review", "bg-purple-500")

	if _, err := svc.RevertPlan(ctx, RevertPlanRequest{
		TaskID: "task-revert-step", TargetRevisionID: v1.ID, AuthorName: "Alice",
	}); err != nil {
		t.Fatalf("revert: %v", err)
	}

	list, _ = svc.ListRevisions(ctx, "task-revert-step")
	if len(list) != 2 {
		t.Fatalf("expected 2 revisions (original + revert), got %d", len(list))
	}
	revertRev := list[0]
	if revertRev.RevertOfRevisionID == nil {
		t.Fatalf("expected the newest revision to be the revert, got %+v", revertRev)
	}
	if revertRev.WorkflowStepID != "step-review" || revertRev.WorkflowStepName != "Review" {
		t.Errorf("expected revert-created revision to carry the current step-review stamp, got %+v", revertRev)
	}
}
