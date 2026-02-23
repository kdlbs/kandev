package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

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

	t.Run("auto_start_agent queues prompt when session is already running", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		step := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "In Progress",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "queued prompt content")

		deadline := time.Now().Add(2 * time.Second)
		for {
			status := svc.messageQueue.GetStatus(ctx, "s1")
			if status.IsQueued {
				if status.Message == nil {
					t.Fatal("expected queued message payload")
				}
				if status.Message.TaskID != "t1" {
					t.Fatalf("expected queued task_id t1, got %s", status.Message.TaskID)
				}
				if status.Message.Content == "" {
					t.Fatal("expected queued content to be populated")
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for auto-start prompt to be queued")
			}
			time.Sleep(10 * time.Millisecond)
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

func TestProcessOnEnterResetAgentContext(t *testing.T) {
	ctx := context.Background()

	t.Run("reset_agent_context calls RestartAgentProcess", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set agent execution ID on the session
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.AgentExecutionID = "exec-123"
		session.Metadata = map[string]interface{}{"acp_session_id": "old-acp-id"}
		_ = repo.UpdateTaskSession(ctx, session)

		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		step := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterResetAgentContext},
				},
			},
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "review task")

		// Verify RestartAgentProcess was called with the correct execution ID
		if len(agentMgr.restartProcessCalls) != 1 {
			t.Fatalf("expected 1 RestartAgentProcess call, got %d", len(agentMgr.restartProcessCalls))
		}
		if agentMgr.restartProcessCalls[0] != "exec-123" {
			t.Errorf("expected RestartAgentProcess called with 'exec-123', got %q", agentMgr.restartProcessCalls[0])
		}

		// Verify acp_session_id was cleared from session metadata
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata != nil {
			if _, hasACP := updated.Metadata["acp_session_id"]; hasACP {
				t.Error("expected acp_session_id to be cleared from session metadata")
			}
		}
	})

	t.Run("reset_agent_context skipped when no execution", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		step := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterResetAgentContext},
				},
			},
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "review task")

		// Verify RestartAgentProcess was NOT called (no execution ID)
		if len(agentMgr.restartProcessCalls) != 0 {
			t.Errorf("expected 0 RestartAgentProcess calls, got %d", len(agentMgr.restartProcessCalls))
		}
	})

	t.Run("reset_agent_context works for passthrough sessions", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set agent execution ID on the session
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.AgentExecutionID = "exec-456"
		_ = repo.UpdateTaskSession(ctx, session)

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		step := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterResetAgentContext},
				},
			},
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "review task")

		// Verify RestartAgentProcess was called even for passthrough sessions
		if len(agentMgr.restartProcessCalls) != 1 {
			t.Fatalf("expected 1 RestartAgentProcess call for passthrough, got %d", len(agentMgr.restartProcessCalls))
		}
		if agentMgr.restartProcessCalls[0] != "exec-456" {
			t.Errorf("expected RestartAgentProcess called with 'exec-456', got %q", agentMgr.restartProcessCalls[0])
		}
	})

	t.Run("reset failure keeps session waiting for input", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.AgentExecutionID = "exec-789"
		_ = repo.UpdateTaskSession(ctx, session)

		agentMgr := &mockAgentManager{restartProcessErr: errors.New("restart failed")}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		step := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterResetAgentContext},
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc.processOnEnter(ctx, "t1", "s1", step, "review task")

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Fatalf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
		}
	})
}
