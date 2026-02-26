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
