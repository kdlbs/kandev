package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
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
		got := svc.processOnTurnComplete(ctx, task, session)
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
		task, _ := repo.GetTask(ctx, "t1")
		session, _ := repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnComplete(ctx, task, session)
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
		task, _ := repo.GetTask(ctx, "t1")
		session, _ := repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnComplete(ctx, task, session)
		if !got {
			t.Error("expected true when move_to_next transitions")
		}

		// Verify the task was updated to step2
		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step2" {
			t.Errorf("expected task workflow step to be 'step2', got %q", updatedTask.WorkflowStepID)
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
		task, _ := repo.GetTask(ctx, "t1")
		session, _ := repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnComplete(ctx, task, session)
		if !got {
			t.Error("expected true when move_to_step transitions")
		}

		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step3" {
			t.Errorf("expected task workflow step to be 'step3', got %q", updatedTask.WorkflowStepID)
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
		task, _ := repo.GetTask(ctx, "t1")
		session, _ := repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnComplete(ctx, task, session)
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
		task, _ := repo.GetTask(ctx, "t1")
		session, _ := repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnComplete(ctx, task, session)
		if got {
			t.Error("expected false when only action requires_approval")
		}

		// Verify task step was NOT changed
		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step1" {
			t.Error("expected task to stay on step1")
		}
	})

	t.Run("disable_plan_mode side-effect with transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"plan_mode": true})

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
		task, _ := repo.GetTask(ctx, "t1")
		session, _ = repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnComplete(ctx, task, session)
		if !got {
			t.Error("expected true when transition occurs alongside disable_plan_mode")
		}

		// Verify plan_mode was cleared
		updatedSession, _ := repo.GetTaskSession(ctx, "s1")
		if updatedSession.Metadata != nil {
			if pm, _ := updatedSession.Metadata["plan_mode"].(bool); pm {
				t.Error("expected plan_mode to be cleared from session metadata")
			}
		}
	})
}

func TestProcessOnTurnStart(t *testing.T) {
	ctx := context.Background()

	t.Run("nil step returns false", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "unknown-step")

		// Step getter returns (nil, nil) for unknown steps — must not panic.
		stepGetter := newMockStepGetter()
		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		task, _ := repo.GetTask(ctx, "t1")
		session, _ := repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnStart(ctx, task, session)
		if got {
			t.Error("expected false when step is nil")
		}
	})

	t.Run("no actions returns false", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{}, // no on_turn_start
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		task, _ := repo.GetTask(ctx, "t1")
		session, _ := repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnStart(ctx, task, session)
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
		task, _ := repo.GetTask(ctx, "t1")
		session, _ := repo.GetTaskSession(ctx, "s1")
		got := svc.processOnTurnStart(ctx, task, session)
		if !got {
			t.Error("expected true when move_to_next transitions")
		}

		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step2" {
			t.Errorf("expected task workflow step to be 'step2', got %q", updatedTask.WorkflowStepID)
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

		session, _ := repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "test task")

		session, _ = repo.GetTaskSession(ctx, "s1")
		if session.Metadata == nil {
			t.Fatal("expected metadata to be set")
		}
		if pm, ok := session.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to be set to true in session metadata")
		}
	})

	t.Run("plan mode persists when entering step without enable_plan_mode", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata (simulates user-initiated plan mode)
		session, _ := repo.GetTaskSession(ctx, "s1")
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"plan_mode": true})

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		step := &wfmodels.WorkflowStep{
			ID: "step1", Name: "Regular Step",
			Events: wfmodels.StepEvents{}, // no enable_plan_mode
		}

		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "test task")

		// Plan mode should persist — only explicit on_exit/on_turn_complete
		// disable_plan_mode actions should clear it.
		updated, _ := repo.GetTaskSession(ctx, "s1")
		pm, _ := updated.Metadata["plan_mode"].(bool)
		if !pm {
			t.Error("expected plan_mode to persist in session metadata")
		}
	})

	// Regression: a workflow transition to a step without auto_start_agent
	// (e.g. Review) must still drain a user-queued message. Pre-#677 the drain
	// happened in handleAgentReady after inline processOnEnter returned;
	// #677 made handleAgentReady early-return on transition, which orphaned
	// the queue for any step that didn't auto-start.
	t.Run("drains user-queued message when entering step without auto_start_agent", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Mirror applyEngineTransition's pre-flip so processOnEnter sees the
		// session in the same state it would in production.
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateWaitingForInput
		session.AgentExecutionID = "exec-1"
		seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
		_ = repo.UpdateTaskSession(ctx, session)

		agentMgr := &mockAgentManager{isAgentRunning: true, promptDone: make(chan struct{})}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
		svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

		if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "user queued msg", "", "user", false, nil); err != nil {
			t.Fatalf("failed to seed queued message: %v", err)
		}

		reviewStep := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review", Position: 1,
			// No on_enter actions — this is the broken case before the fix.
		}

		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, reviewStep, "task description")

		select {
		case <-agentMgr.promptDone:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for queued message to be sent to agent")
		}

		agentMgr.mu.Lock()
		got := agentMgr.capturedPrompts[0]
		agentMgr.mu.Unlock()
		if got != "user queued msg" {
			t.Fatalf("agent received %q, want %q", got, "user queued msg")
		}

		if status := svc.messageQueue.GetStatus(ctx, "s1"); status.IsQueued {
			t.Fatalf("expected queue to be drained, still queued: %+v", status.Message)
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

		session, _ := repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "queued prompt content")

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
		session, _ := repo.GetTaskSession(ctx, "s1")
		svc.setSessionPlanMode(ctx, session, true)

		session, _ = repo.GetTaskSession(ctx, "s1")
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
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"plan_mode": true})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.setSessionPlanMode(ctx, session, false)

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata != nil {
			if pm, _ := updated.Metadata["plan_mode"].(bool); pm {
				t.Error("expected plan_mode to be removed from metadata")
			}
		}
	})

	t.Run("nil metadata gets initialized", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		session, _ := repo.GetTaskSession(ctx, "s1")
		svc.setSessionPlanMode(ctx, session, true)

		session, _ = repo.GetTaskSession(ctx, "s1")
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

		session, _ := repo.GetTaskSession(ctx, "s1")
		step := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1",
			Events: wfmodels.StepEvents{},
		}

		// Should not panic or modify anything
		svc.processOnExit(ctx, "t1", session, step)
	})

	t.Run("disable_plan_mode clears plan mode", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"plan_mode": true})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		step := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1",
			Events: wfmodels.StepEvents{
				OnExit: []wfmodels.OnExitAction{
					{Type: wfmodels.OnExitDisablePlanMode},
				},
			},
		}

		svc.processOnExit(ctx, "t1", session, step)

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata != nil {
			if pm, _ := updated.Metadata["plan_mode"].(bool); pm {
				t.Error("expected plan_mode to be cleared from session metadata")
			}
		}
	})

	t.Run("disable_plan_mode skipped for passthrough session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"plan_mode": true})

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isPassthrough: true})

		step := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1",
			Events: wfmodels.StepEvents{
				OnExit: []wfmodels.OnExitAction{
					{Type: wfmodels.OnExitDisablePlanMode},
				},
			},
		}

		svc.processOnExit(ctx, "t1", session, step)

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

func TestSetSessionPlanModeByID(t *testing.T) {
	ctx := context.Background()

	t.Run("clears plan mode when enabled=false", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		_ = repo.UpdateSessionMetadata(ctx, "s1", map[string]interface{}{"plan_mode": true})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		if err := svc.SetSessionPlanModeByID(ctx, "s1", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if pm, _ := updated.Metadata["plan_mode"].(bool); pm {
			t.Error("expected plan_mode to be cleared")
		}
	})

	t.Run("sets plan mode when enabled=true", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		if err := svc.SetSessionPlanModeByID(ctx, "s1", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if pm, ok := updated.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to be true")
		}
	})

	t.Run("no-op for passthrough session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		_ = repo.UpdateSessionMetadata(ctx, "s1", map[string]interface{}{"plan_mode": true})

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isPassthrough: true})

		if err := svc.SetSessionPlanModeByID(ctx, "s1", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if pm, ok := updated.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to remain true for passthrough session")
		}
	})

	t.Run("propagates session lookup error", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		if err := svc.SetSessionPlanModeByID(ctx, "missing", false); err == nil {
			t.Error("expected error for missing session")
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

		session, _ := repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "test task")

		session, _ = repo.GetTaskSession(ctx, "s1")
		if session.Metadata != nil {
			if pm, _ := session.Metadata["plan_mode"].(bool); pm {
				t.Error("expected plan_mode NOT to be set for passthrough session")
			}
		}
	})

	t.Run("plan mode not cleared for passthrough session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode in session metadata
		session, _ := repo.GetTaskSession(ctx, "s1")
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"plan_mode": true})

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isPassthrough: true})

		step := &wfmodels.WorkflowStep{
			ID: "step1", Name: "Regular Step",
			Events: wfmodels.StepEvents{}, // no enable_plan_mode
		}

		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "test task")

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
		seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-123")
		session.Metadata = map[string]interface{}{"acp_session_id": "old-acp-id"}
		_ = repo.UpdateTaskSession(ctx, session)

		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		step := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterResetAgentContext},
				},
			},
		}

		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "review task")

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
			if acp, _ := updated.Metadata["acp_session_id"].(string); acp != "" {
				t.Error("expected acp_session_id to be cleared from session metadata")
			}
		}
	})

	t.Run("reset_agent_context skipped when no execution", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		step := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterResetAgentContext},
				},
			},
		}

		session, _ := repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "review task")

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
		seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-456")
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

		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "review task")

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
		seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-789")
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

		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "review task")

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Fatalf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
		}
	})

	// Regression: on_turn_complete auto-transition enters processOnEnter with an
	// in-memory session still marked RUNNING (loaded at the start of
	// handleAgentReady before the turn finished). After reset_agent_context we
	// must flip the session to WAITING_FOR_INPUT — otherwise a following
	// auto_start_agent hits queueAutoStartPromptIfRunning (sees State=RUNNING)
	// and queues the prompt instead of sending it, and PromptTask rejects the
	// drained queued message for the same reason. The agent then sits idle
	// until the user cancels. Both the in-memory pointer and the DB must be
	// updated.
	t.Run("reset_agent_context flips session to WAITING_FOR_INPUT for a running session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1") // seeds session.State = RUNNING

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.AgentExecutionID = "exec-abc"
		seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-abc")
		session.Metadata = map[string]interface{}{"acp_session_id": "old-acp"}
		_ = repo.UpdateTaskSession(ctx, session)

		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		step := &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review Step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterResetAgentContext},
				},
			},
		}

		session, _ = repo.GetTaskSession(ctx, "s1")
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("precondition: seed should start session as RUNNING, got %q", session.State)
		}

		svc.processOnEnter(ctx, "t1", session, step, "review task")

		// In-memory session must also be updated — queueAutoStartPromptIfRunning
		// (further down in processOnEnter when auto_start_agent is present)
		// checks the pointer, not the DB.
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("in-memory session.State: want %q, got %q",
				models.TaskSessionStateWaitingForInput, session.State)
		}

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("DB session.State: want %q, got %q",
				models.TaskSessionStateWaitingForInput, updated.State)
		}

		if len(agentMgr.restartProcessCalls) != 1 {
			t.Errorf("expected 1 RestartAgentProcess call, got %d", len(agentMgr.restartProcessCalls))
		}
	})

	// Regression: passthrough sessions handle auto_start_agent by writing to
	// PTY stdin via autoStartPassthroughPrompt — the agent is actively
	// processing immediately after, not idle. We must NOT flip the session to
	// WAITING_FOR_INPUT, otherwise WS subscribers see "ready for input" while
	// the agent is mid-prompt.
	t.Run("reset_agent_context preserves running state for passthrough+auto_start", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1") // RUNNING

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.AgentExecutionID = "exec-pt"
		seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-pt")
		_ = repo.UpdateTaskSession(ctx, session)

		agentMgr := &mockAgentManager{isPassthrough: true}
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

		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.processOnEnter(ctx, "t1", session, step, "review task")

		// Positive assertion: pin the expected state to RUNNING so any other
		// unintended mutation (COMPLETED, FAILED, etc.) also fails the test,
		// not just an erroneous flip to WAITING_FOR_INPUT.
		if session.State != models.TaskSessionStateRunning {
			t.Errorf("in-memory session.State should remain RUNNING for passthrough+auto_start, got %q", session.State)
		}
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateRunning {
			t.Errorf("DB session.State should remain RUNNING for passthrough+auto_start, got %q", updated.State)
		}
	})
}
