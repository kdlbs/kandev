package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// --- Mocks ---

// mockStepGetter implements WorkflowStepGetter for testing.
type mockStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep // stepID -> step
}

func newMockStepGetter() *mockStepGetter {
	return &mockStepGetter{steps: make(map[string]*wfmodels.WorkflowStep)}
}

func (m *mockStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if s, ok := m.steps[stepID]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockStepGetter) GetNextStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	var best *wfmodels.WorkflowStep
	for _, s := range m.steps {
		if s.WorkflowID == workflowID && s.Position > currentPosition {
			if best == nil || s.Position < best.Position {
				best = s
			}
		}
	}
	return best, nil
}

func (m *mockStepGetter) GetPreviousStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	var best *wfmodels.WorkflowStep
	for _, s := range m.steps {
		if s.WorkflowID == workflowID && s.Position < currentPosition {
			if best == nil || s.Position > best.Position {
				best = s
			}
		}
	}
	return best, nil
}

// mockTaskRepo implements scheduler.TaskRepository for testing.
type mockTaskRepo struct {
	tasks         map[string]*v1.Task
	updatedStates map[string]v1.TaskState
}

func newMockTaskRepo() *mockTaskRepo {
	return &mockTaskRepo{
		tasks:         make(map[string]*v1.Task),
		updatedStates: make(map[string]v1.TaskState),
	}
}

func (m *mockTaskRepo) GetTask(_ context.Context, taskID string) (*v1.Task, error) {
	if t, ok := m.tasks[taskID]; ok {
		return t, nil
	}
	return nil, nil
}

func (m *mockTaskRepo) UpdateTaskState(_ context.Context, taskID string, state v1.TaskState) error {
	m.updatedStates[taskID] = state
	return nil
}

// mockAgentManager is a minimal mock of executor.AgentManagerClient for testing.
type mockAgentManager struct {
	isPassthrough bool
}

func (m *mockAgentManager) LaunchAgent(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	return nil, nil
}
func (m *mockAgentManager) StartAgentProcess(_ context.Context, _ string) error { return nil }
func (m *mockAgentManager) StopAgent(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockAgentManager) PromptAgent(_ context.Context, _ string, _ string, _ []v1.MessageAttachment) (*executor.PromptResult, error) {
	return nil, nil
}
func (m *mockAgentManager) CancelAgent(_ context.Context, _ string) error { return nil }
func (m *mockAgentManager) RespondToPermissionBySessionID(_ context.Context, _, _, _ string, _ bool) error {
	return nil
}
func (m *mockAgentManager) IsAgentRunningForSession(_ context.Context, _ string) bool { return false }
func (m *mockAgentManager) ResolveAgentProfile(_ context.Context, _ string) (*executor.AgentProfileInfo, error) {
	return nil, nil
}
func (m *mockAgentManager) SetExecutionDescription(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockAgentManager) IsPassthroughSession(_ context.Context, _ string) bool {
	return m.isPassthrough
}

// --- Helpers ---

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "console",
	})
	return log
}

func strPtr(s string) *string { return &s }

// setupTestRepo creates a real in-memory SQLite repository for testing.
func setupTestRepo(t *testing.T) repository.Repository {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	repo, cleanup, err := repository.Provide(sqlxDB)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	return repo
}

// seedSession creates a task, workspace, workflow and session in the repo for testing.
func seedSession(t *testing.T, repo repository.Repository, taskID, sessionID, workflowStepID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	// Create workspace
	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create workflow
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test Workflow", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		// Might already exist
		_ = err
	}

	// Create task
	task := &models.Task{
		ID:             taskID,
		WorkflowID:     "wf1",
		WorkflowStepID: workflowStepID,
		Title:          "Test Task",
		Description:    "Test",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Create session
	session := &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		State:          models.TaskSessionStateRunning,
		WorkflowStepID: strPtr(workflowStepID),
		StartedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create task session: %v", err)
	}
}

// createTestService creates a Service with minimal dependencies for event handler testing.
func createTestService(repo repository.Repository, stepGetter *mockStepGetter, taskRepo *mockTaskRepo) *Service {
	return createTestServiceWithAgent(repo, stepGetter, taskRepo, &mockAgentManager{})
}

func createTestServiceWithAgent(repo repository.Repository, stepGetter *mockStepGetter, taskRepo *mockTaskRepo, agentMgr executor.AgentManagerClient) *Service {
	return &Service{
		logger:             testLogger(),
		repo:               repo,
		workflowStepGetter: stepGetter,
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
	}
}

// --- Tests ---

func TestIsResumeFailure(t *testing.T) {
	tests := []struct {
		name     string
		errorMsg string
		want     bool
	}{
		{name: "exact match", errorMsg: "no conversation found", want: true},
		{name: "mixed case", errorMsg: "No Conversation Found", want: true},
		{name: "embedded in longer message", errorMsg: "prompt failed: no conversation found for session abc-123", want: true},
		{name: "unrelated error", errorMsg: "connection refused", want: false},
		{name: "empty string", errorMsg: "", want: false},
		{name: "agent crashed", errorMsg: "agent process exited with code 1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isResumeFailure(tt.errorMsg)
			if got != tt.want {
				t.Errorf("isResumeFailure(%q) = %v, want %v", tt.errorMsg, got, tt.want)
			}
		})
	}
}

func TestProcessOnTurnComplete(t *testing.T) {
	ctx := context.Background()

	t.Run("no session step returns false", func(t *testing.T) {
		repo := setupTestRepo(t)
		// Create session without workflow step
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateTask(ctx, task)
		session := &models.TaskSession{ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}
		_ = repo.CreateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		got := svc.processOnTurnComplete(ctx, "t1", "s1")
		if got {
			t.Error("expected false when session has no workflow step")
		}
	})

	t.Run("no actions returns false", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{}, // no actions
		}

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, stepGetter, taskRepo)
		got := svc.processOnTurnComplete(ctx, "t1", "s1")
		if got {
			t.Error("expected false when step has no on_turn_complete actions")
		}
	})

	t.Run("move_to_next transitions to next step", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{},
		}

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, stepGetter, taskRepo)
		got := svc.processOnTurnComplete(ctx, "t1", "s1")
		if !got {
			t.Error("expected true when move_to_next transitions")
		}

		// Verify the session was updated to step2
		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.WorkflowStepID == nil || *session.WorkflowStepID != "step2" {
			t.Errorf("expected session workflow step to be 'step2', got %v", session.WorkflowStepID)
		}
	})

	t.Run("move_to_step transitions to specified step", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step3"}},
				},
			},
		}
		stepGetter.steps["step3"] = &wfmodels.WorkflowStep{
			ID: "step3", WorkflowID: "wf1", Name: "Step 3", Position: 2,
			Events: wfmodels.StepEvents{},
		}

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, stepGetter, taskRepo)
		got := svc.processOnTurnComplete(ctx, "t1", "s1")
		if !got {
			t.Error("expected true when move_to_step transitions")
		}

		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.WorkflowStepID == nil || *session.WorkflowStepID != "step3" {
			t.Errorf("expected session workflow step to be 'step3', got %v", session.WorkflowStepID)
		}
	})

	t.Run("last step with move_to_next stays", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step_last")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step_last"] = &wfmodels.WorkflowStep{
			ID: "step_last", WorkflowID: "wf1", Name: "Last Step", Position: 99,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, stepGetter, taskRepo)
		got := svc.processOnTurnComplete(ctx, "t1", "s1")
		if got {
			t.Error("expected false when at last step with move_to_next (no next step)")
		}
	})

	t.Run("requires_approval action is skipped", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{
						Type: wfmodels.OnTurnCompleteMoveToStep,
						Config: map[string]interface{}{
							"step_id":           "step2",
							"requires_approval": true,
						},
					},
				},
			},
		}

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, stepGetter, taskRepo)
		got := svc.processOnTurnComplete(ctx, "t1", "s1")
		if got {
			t.Error("expected false when only action requires_approval")
		}

		// Verify session step was NOT changed
		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.WorkflowStepID == nil || *session.WorkflowStepID != "step1" {
			t.Error("expected session to stay on step1")
		}
	})

	t.Run("disable_plan_mode side-effect with transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.Metadata = map[string]interface{}{"plan_mode": true}
		_ = repo.UpdateTaskSession(ctx, session)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteDisablePlanMode},
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{},
		}

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, stepGetter, taskRepo)
		got := svc.processOnTurnComplete(ctx, "t1", "s1")
		if !got {
			t.Error("expected true when transition occurs alongside disable_plan_mode")
		}

		// Verify plan_mode was cleared
		updatedSession, _ := repo.GetTaskSession(ctx, "s1")
		if updatedSession.Metadata != nil {
			if _, hasPlanMode := updatedSession.Metadata["plan_mode"]; hasPlanMode {
				t.Error("expected plan_mode to be cleared from session metadata")
			}
		}
	})
}

func TestProcessOnTurnStart(t *testing.T) {
	ctx := context.Background()

	t.Run("no actions returns false", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{}, // no on_turn_start
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		got := svc.processOnTurnStart(ctx, "t1", "s1")
		if got {
			t.Error("expected false when step has no on_turn_start actions")
		}
	})

	t.Run("move_to_next transitions", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnStart: []wfmodels.OnTurnStartAction{
					{Type: wfmodels.OnTurnStartMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{},
		}

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, stepGetter, taskRepo)
		got := svc.processOnTurnStart(ctx, "t1", "s1")
		if !got {
			t.Error("expected true when move_to_next transitions")
		}

		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.WorkflowStepID == nil || *session.WorkflowStepID != "step2" {
			t.Errorf("expected session workflow step to be 'step2', got %v", session.WorkflowStepID)
		}
	})
}

func TestProcessOnEnter(t *testing.T) {
	ctx := context.Background()

	t.Run("enable_plan_mode sets plan mode", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		step := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Plan Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterEnablePlanMode},
				},
			},
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "test task")

		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.Metadata == nil {
			t.Fatal("expected metadata to be set")
		}
		if pm, ok := session.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to be set to true in session metadata")
		}
	})

	t.Run("no plan mode clears it", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.Metadata = map[string]interface{}{"plan_mode": true}
		_ = repo.UpdateTaskSession(ctx, session)

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		step := &wfmodels.WorkflowStep{
			ID: "step1", Name: "Regular Step",
			Events: wfmodels.StepEvents{}, // no enable_plan_mode
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "test task")

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata != nil {
			if _, hasPlanMode := updated.Metadata["plan_mode"]; hasPlanMode {
				t.Error("expected plan_mode to be cleared from session metadata")
			}
		}
	})
}

func TestSetSessionPlanMode(t *testing.T) {
	ctx := context.Background()

	t.Run("enables plan mode", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.setSessionPlanMode(ctx, "s1", true)

		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.Metadata == nil {
			t.Fatal("expected metadata to be set")
		}
		if pm, ok := session.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to be true")
		}
	})

	t.Run("disables plan mode", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// First enable
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.Metadata = map[string]interface{}{"plan_mode": true}
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.setSessionPlanMode(ctx, "s1", false)

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata != nil {
			if _, hasPlanMode := updated.Metadata["plan_mode"]; hasPlanMode {
				t.Error("expected plan_mode to be removed from metadata")
			}
		}
	})

	t.Run("nil metadata gets initialized", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.setSessionPlanMode(ctx, "s1", true)

		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.Metadata == nil {
			t.Fatal("expected metadata to be initialized")
		}
		if pm, ok := session.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to be true after initialization")
		}
	})
}

func TestProcessOnExit(t *testing.T) {
	ctx := context.Background()

	t.Run("no actions is a no-op", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		step := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1",
			Events: wfmodels.StepEvents{},
		}

		// Should not panic or modify anything
		svc.processOnExit(ctx, "t1", "s1", step)
	})

	t.Run("disable_plan_mode clears plan mode", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.Metadata = map[string]interface{}{"plan_mode": true}
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		step := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1",
			Events: wfmodels.StepEvents{
				OnExit: []wfmodels.OnExitAction{
					{Type: wfmodels.OnExitDisablePlanMode},
				},
			},
		}

		svc.processOnExit(ctx, "t1", "s1", step)

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata != nil {
			if _, hasPlanMode := updated.Metadata["plan_mode"]; hasPlanMode {
				t.Error("expected plan_mode to be cleared from session metadata")
			}
		}
	})

	t.Run("disable_plan_mode skipped for passthrough session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.Metadata = map[string]interface{}{"plan_mode": true}
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isPassthrough: true})

		step := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1",
			Events: wfmodels.StepEvents{
				OnExit: []wfmodels.OnExitAction{
					{Type: wfmodels.OnExitDisablePlanMode},
				},
			},
		}

		svc.processOnExit(ctx, "t1", "s1", step)

		// plan_mode should still be set
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata == nil {
			t.Fatal("expected metadata to still be set")
		}
		if pm, ok := updated.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to remain true for passthrough session")
		}
	})
}

func TestProcessOnEnterPassthrough(t *testing.T) {
	ctx := context.Background()

	t.Run("plan mode not set for passthrough session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isPassthrough: true})

		step := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Plan Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterEnablePlanMode},
				},
			},
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "test task")

		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.Metadata != nil {
			if _, hasPlanMode := session.Metadata["plan_mode"]; hasPlanMode {
				t.Error("expected plan_mode NOT to be set for passthrough session")
			}
		}
	})

	t.Run("plan mode not cleared for passthrough session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.Metadata = map[string]interface{}{"plan_mode": true}
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isPassthrough: true})

		step := &wfmodels.WorkflowStep{
			ID: "step1", Name: "Regular Step",
			Events: wfmodels.StepEvents{}, // no enable_plan_mode
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "test task")

		// plan_mode should still be set since passthrough sessions skip plan mode management
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata == nil {
			t.Fatal("expected metadata to still be set")
		}
		if pm, ok := updated.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to remain true for passthrough session")
		}
	})
}

