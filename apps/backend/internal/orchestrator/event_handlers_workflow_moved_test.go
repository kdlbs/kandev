package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestHandleTaskMoved(t *testing.T) {
	ctx := context.Background()

	t.Run("skips when fromStepID is empty", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// Should not panic or modify anything
		svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
			TaskID:     "t1",
			FromStepID: "",
			ToStepID:   "step2",
			SessionID:  "s1",
		})
	})

	t.Run("skips when toStepID is empty", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
			TaskID:     "t1",
			FromStepID: "step1",
			ToStepID:   "",
			SessionID:  "s1",
		})
	})

	t.Run("skips when workflowStepGetter is nil", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.workflowStepGetter = nil

		svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
			TaskID:     "t1",
			FromStepID: "step1",
			ToStepID:   "step2",
			SessionID:  "s1",
		})
	})

	t.Run("routes to no-session handler when sessionID is empty", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{}, // no auto_start
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		// Should not panic — with no auto-start on the step, it just logs and returns
		svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
			TaskID:     "t1",
			FromStepID: "step1",
			ToStepID:   "step2",
			SessionID:  "", // no session
		})
	})
}

func TestHandleTaskMovedNoSession(t *testing.T) {
	ctx := context.Background()

	t.Run("skips when target step has no auto_start", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{}, // no on_enter actions
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
			TaskID:     "t1",
			FromStepID: "step1",
			ToStepID:   "step2",
		})

		// No crash, no new sessions created
	})

	t.Run("skips when target step has non-auto-start on_enter", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterEnablePlanMode}, // not auto_start
				},
			},
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
			TaskID:     "t1",
			FromStepID: "step1",
			ToStepID:   "step2",
		})
	})

	t.Run("skips auto-start when task load fails", func(t *testing.T) {
		repo := setupTestRepo(t)
		// Don't seed a task — simulate task not found
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Auto Start Step", Position: 1,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		// Should not panic — task not found is logged and returns
		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
			TaskID:     "nonexistent",
			FromStepID: "step1",
			ToStepID:   "step2",
		})
	})
}

func TestHandleTaskMovedWithSession(t *testing.T) {
	ctx := context.Background()

	t.Run("processes on_exit and on_enter for step transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode on the session to verify on_exit clears it
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.Metadata = map[string]interface{}{"plan_mode": true}
		_ = repo.UpdateTaskSession(ctx, session)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnExit: []wfmodels.OnExitAction{
					{Type: wfmodels.OnExitDisablePlanMode},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterEnablePlanMode},
				},
			},
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		svc.handleTaskMovedWithSession(ctx, watcher.TaskMovedEventData{
			TaskID:          "t1",
			SessionID:       "s1",
			FromStepID:      "step1",
			ToStepID:        "step2",
			TaskDescription: "test task",
		})

		// Verify on_exit cleared plan_mode, then on_enter re-enabled it
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata == nil {
			t.Fatal("expected metadata to be set")
		}
		if pm, ok := updated.Metadata["plan_mode"].(bool); !ok || !pm {
			t.Error("expected plan_mode to be true after on_enter re-enabled it")
		}
	})

	t.Run("clears review status on step transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set review status on the session
		_ = repo.UpdateSessionReviewStatus(ctx, "s1", "pending")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{},
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		svc.handleTaskMovedWithSession(ctx, watcher.TaskMovedEventData{
			TaskID:          "t1",
			SessionID:       "s1",
			FromStepID:      "step1",
			ToStepID:        "step2",
			TaskDescription: "test task",
		})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.ReviewStatus != nil && *updated.ReviewStatus != "" {
			t.Errorf("expected review status to be cleared, got %q", *updated.ReviewStatus)
		}
	})

	t.Run("queues auto-start prompt on enter", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "In Progress", Position: 1,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		svc.handleTaskMovedWithSession(ctx, watcher.TaskMovedEventData{
			TaskID:          "t1",
			SessionID:       "s1",
			FromStepID:      "step1",
			ToStepID:        "step2",
			TaskDescription: "auto-start task",
		})

		// Since session is in RUNNING state, the auto-start prompt should be queued
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
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for auto-start prompt to be queued")
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	t.Run("does not pre-queue when session not in RUNNING state", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set session to CREATED (not RUNNING) — the pre-check should NOT queue
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateCreated
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		queued, err := svc.queueAutoStartPromptIfRunning(ctx, "t1", session, "prompt", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if queued {
			t.Error("expected pre-check NOT to queue when session state is CREATED")
		}
	})

	t.Run("handles missing from-step gracefully", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		// step1 intentionally NOT in stepGetter
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{},
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		// Should not panic — from-step lookup failure is logged but processing continues
		svc.handleTaskMovedWithSession(ctx, watcher.TaskMovedEventData{
			TaskID:          "t1",
			SessionID:       "s1",
			FromStepID:      "nonexistent",
			ToStepID:        "step2",
			TaskDescription: "test task",
		})
	})

	t.Run("handles missing to-step gracefully", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{},
		}
		// step2 intentionally NOT in stepGetter

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		// Should not panic — to-step lookup failure causes early return
		svc.handleTaskMovedWithSession(ctx, watcher.TaskMovedEventData{
			TaskID:          "t1",
			SessionID:       "s1",
			FromStepID:      "step1",
			ToStepID:        "nonexistent",
			TaskDescription: "test task",
		})
	})

	t.Run("reset_agent_context processed on enter", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set agent execution ID on the session
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.AgentExecutionID = "exec-123"
		session.Metadata = map[string]interface{}{"acp_session_id": "old-acp"}
		_ = repo.UpdateTaskSession(ctx, session)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Review", Position: 1,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterResetAgentContext},
				},
			},
		}

		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentMgr)
		svc.handleTaskMovedWithSession(ctx, watcher.TaskMovedEventData{
			TaskID:          "t1",
			SessionID:       "s1",
			FromStepID:      "step1",
			ToStepID:        "step2",
			TaskDescription: "test task",
		})

		if len(agentMgr.restartProcessCalls) != 1 {
			t.Fatalf("expected 1 RestartAgentProcess call, got %d", len(agentMgr.restartProcessCalls))
		}
		if agentMgr.restartProcessCalls[0] != "exec-123" {
			t.Errorf("expected RestartAgentProcess called with 'exec-123', got %q", agentMgr.restartProcessCalls[0])
		}

		// Verify acp_session_id was cleared
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata != nil {
			if _, hasACP := updated.Metadata["acp_session_id"]; hasACP {
				t.Error("expected acp_session_id to be cleared from session metadata")
			}
		}
	})
}

func TestProcessStepExitAndEnter(t *testing.T) {
	ctx := context.Background()

	t.Run("runs on_exit then on_enter", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode on session — on_exit should clear it
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.Metadata = map[string]interface{}{"plan_mode": true}
		_ = repo.UpdateTaskSession(ctx, session)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnExit: []wfmodels.OnExitAction{
					{Type: wfmodels.OnExitDisablePlanMode},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{}, // no on_enter
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		session, _ = repo.GetTaskSession(ctx, "s1")
		svc.processStepExitAndEnter(ctx, "t1", session, "step1", "step2", "test task")

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.Metadata != nil {
			if _, hasPlanMode := updated.Metadata["plan_mode"]; hasPlanMode {
				t.Error("expected plan_mode to be cleared by on_exit")
			}
		}
	})

	t.Run("handles missing from-step", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		// from-step not registered
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
			Events: wfmodels.StepEvents{},
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		session, _ := repo.GetTaskSession(ctx, "s1")
		// Should not panic
		svc.processStepExitAndEnter(ctx, "t1", session, "nonexistent", "step2", "test task")
	})

	t.Run("handles missing to-step", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{},
		}
		// to-step not registered

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		session, _ := repo.GetTaskSession(ctx, "s1")
		// Should not panic
		svc.processStepExitAndEnter(ctx, "t1", session, "step1", "nonexistent", "test task")
	})
}
