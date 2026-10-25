package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
func TestService_FeederPromotionCarriesPrimarySession(t *testing.T) {
	svc, eventBus, repo := setupFeederPromotionSessionTest(t)
	ctx := context.Background()
	createMoveSession(t, ctx, repo, "session-primary", "task-promoted", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	newer := time.Now().UTC().Add(time.Minute)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-newer", TaskID: "task-promoted", State: models.TaskSessionStateWaitingForInput,
		StartedAt: newer, UpdatedAt: newer,
	}); err != nil {
		t.Fatalf("CreateTaskSession(session-newer): %v", err)
	}
	eventBus.ClearEvents()

	if _, err := svc.MoveTask(ctx, "task-vacating", "wf-target", "step-target", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.TaskMoved {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok || data["task_id"] != "task-promoted" {
			continue
		}
		if got := data["session_id"]; got != "session-primary" {
			t.Fatalf("promoted task session_id = %q, want session-primary", got)
		}
		return
	}
	t.Fatal("promoted task.moved event not published")
}

// @covers AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
func TestService_FeederPromotionSkipsCandidateWhenSessionLookupFails(t *testing.T) {
	svc, eventBus, repo := setupFeederPromotionSessionTest(t)
	svc.sessions = &failingListTaskSessionsRepository{
		SessionRepository: repo,
		err:               errors.New("session list failed"),
		taskID:            "task-promoted",
	}
	eventBus.ClearEvents()

	if _, err := svc.MoveTask(context.Background(), "task-vacating", "wf-target", "step-target", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	promoted, err := repo.GetTask(context.Background(), "task-promoted")
	if err != nil {
		t.Fatalf("GetTask(task-promoted): %v", err)
	}
	if promoted.WorkflowStepID != "step-feeder" {
		t.Fatalf("promoted task step = %q, want step-feeder", promoted.WorkflowStepID)
	}
	for _, event := range eventBus.GetPublishedEvents() {
		data, ok := event.Data.(map[string]interface{})
		if event.Type == events.TaskMoved && ok && data["task_id"] == "task-promoted" {
			t.Fatal("session lookup failure published a feeder promotion")
		}
	}
}

// @covers AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
func TestService_FeederPromotionBlocksOnOlderRunningSession(t *testing.T) {
	svc, eventBus, repo := setupFeederPromotionSessionTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-primary", TaskID: "task-promoted", State: models.TaskSessionStateWaitingForInput,
		StartedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute), IsPrimary: true,
	}); err != nil {
		t.Fatalf("CreateTaskSession(session-primary): %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-running", TaskID: "task-promoted", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession(session-running): %v", err)
	}
	eventBus.ClearEvents()

	if _, err := svc.MoveTask(ctx, "task-vacating", "wf-target", "step-target", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	promoted, err := repo.GetTask(ctx, "task-promoted")
	if err != nil {
		t.Fatalf("GetTask(task-promoted): %v", err)
	}
	if promoted.WorkflowStepID != "step-feeder" {
		t.Fatalf("promoted task step = %q, want step-feeder", promoted.WorkflowStepID)
	}
}

// @covers AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
func TestService_FeederPromotionUsesNewestActiveFallback(t *testing.T) {
	svc, eventBus, repo := setupFeederPromotionSessionTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, session := range []*models.TaskSession{
		{ID: "session-older", TaskID: "task-promoted", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now},
		{ID: "session-newer", TaskID: "task-promoted", State: models.TaskSessionStateWaitingForInput, StartedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	eventBus.ClearEvents()

	if _, err := svc.MoveTask(ctx, "task-vacating", "wf-target", "step-target", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.TaskMoved {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok || data["task_id"] != "task-promoted" {
			continue
		}
		if got := data["session_id"]; got != "session-newer" {
			t.Fatalf("promoted task session_id = %q, want session-newer", got)
		}
		return
	}
	t.Fatal("promoted task.moved event not published")
}

func setupFeederPromotionSessionTest(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-limited": {
			ID: "step-limited", WorkflowID: "wf-source", Name: "Limited", Position: 0,
			WIPLimit: 1, PullFromStepID: "step-feeder",
		},
		"step-feeder": {ID: "step-feeder", WorkflowID: "wf-source", Name: "Feeder", Position: 1},
		"step-target": {ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0},
	}})
	createMoveTask(t, ctx, repo, "task-vacating", "wf-source", "step-limited", nil)
	createMoveTask(t, ctx, repo, "task-promoted", "wf-source", "step-feeder", nil)
	return svc, eventBus, repo
}

type failingListTaskSessionsRepository struct {
	repository.SessionRepository
	err    error
	taskID string
}

func (r *failingListTaskSessionsRepository) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if taskID == r.taskID {
		return nil, r.err
	}
	return r.SessionRepository.ListTaskSessions(ctx, taskID)
}
