package orchestrator

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

	t.Run("auto-start uses executor_id from task metadata", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		metadata := map[string]interface{}{
			models.MetaKeyAgentProfileID: "profile-1",
			models.MetaKeyExecutorID:     "exec-special",
		}
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:             "t1",
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step1",
			Title:          "Task",
			Description:    "prompt",
			State:          v1.TaskStateCreated,
			Metadata:       metadata,
			CreatedAt:      now,
			UpdatedAt:      now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Auto Start Step", Position: 1, AgentProfileID: "profile-step",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		taskRepo := newMockTaskRepo()
		taskRepo.tasks["t1"] = &v1.Task{
			ID:          "t1",
			WorkspaceID: "ws1",
			WorkflowID:  "wf1",
			Description: "prompt",
			State:       v1.TaskStateCreated,
			Metadata:    metadata,
		}
		launched := make(chan string, 1)
		agentMgr := &mockAgentManager{
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				executorID, _ := req.Metadata[models.MetaKeyExecutorID].(string)
				launched <- executorID
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
			},
		}
		svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
			TaskID:   "t1",
			ToStepID: "step2",
		})

		select {
		case got := <-launched:
			if got != "exec-special" {
				t.Fatalf("ExecutorID = %q, want exec-special", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for auto-start launch")
		}

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			sessions, err := repo.ListTaskSessions(ctx, "t1")
			if err != nil {
				t.Fatalf("ListTaskSessions: %v", err)
			}
			for _, session := range sessions {
				if session.Metadata[models.SessionMetaKeyCreatedBy] == models.SessionCreatedByWorkflowSwitch {
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("timed out waiting for workflow-routed session metadata")
	})
}

func TestHandleTaskMovedWithSession(t *testing.T) {
	ctx := context.Background()

	t.Run("processes on_exit and on_enter for step transition", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			// Set plan_mode on the session to verify on_exit clears it
			session, _ := repo.GetTaskSession(ctx, "s1")
			_ = repo.UpdateTaskSession(ctx, session)
			_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"plan_mode": true})

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

			synctest.Wait()

			// Verify on_exit cleared plan_mode, then on_enter re-enabled it
			updated, _ := repo.GetTaskSession(ctx, "s1")
			if updated.Metadata == nil {
				t.Fatal("expected metadata to be set")
			}
			if pm, ok := updated.Metadata["plan_mode"].(bool); !ok || !pm {
				t.Error("expected plan_mode to be true after on_enter re-enabled it")
			}
		})
	})

	t.Run("preserves review status on manual move to non-auto-start step", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
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

			synctest.Wait()

			updated, _ := repo.GetTaskSession(ctx, "s1")
			if updated.ReviewStatus != models.ReviewStatusPending {
				t.Fatalf("expected pending review status to be preserved, got %#v", updated.ReviewStatus)
			}
		})
	})

	t.Run("clears review status on manual move to auto-start step", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			_ = repo.UpdateSessionReviewStatus(ctx, "s1", "pending")

			stepGetter := newMockStepGetter()
			stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
				ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
				Events: wfmodels.StepEvents{},
			}
			stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
				ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
				Events: wfmodels.StepEvents{
					OnEnter: []wfmodels.OnEnterAction{
						{Type: wfmodels.OnEnterAutoStartAgent},
					},
				},
			}

			svc := createTestService(repo, stepGetter, newMockTaskRepo())
			// Seed an active-turn entry so flipStaleRunningToWaiting treats the
			// session as genuinely mid-turn and leaves auto-start on the queue
			// path. Without this, the helper would flip state to WAITING_FOR_INPUT
			// and autoStartStepPrompt would fall through to PromptTask (which
			// dereferences the executor that's nil in this minimal test setup).
			svc.activeTurns.Store("s1", "turn-1")
			svc.handleTaskMovedWithSession(ctx, watcher.TaskMovedEventData{
				TaskID:          "t1",
				SessionID:       "s1",
				FromStepID:      "step1",
				ToStepID:        "step2",
				TaskDescription: "test task",
			})

			synctest.Wait()

			updated, _ := repo.GetTaskSession(ctx, "s1")
			if updated.ReviewStatus != models.ReviewStatusNone {
				t.Fatalf("expected review status to be cleared, got %q", updated.ReviewStatus)
			}
		})
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
			ID: "step2", WorkflowID: "wf1", Name: "In Progress", Position: 1, Color: "bg-emerald-500",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		// Seed an active-turn entry so flipStaleRunningToWaiting recognises the
		// session as genuinely mid-turn and the auto-start prompt is queued
		// (the behavior this subtest asserts).
		svc.activeTurns.Store("s1", "turn-1")
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
			if status.Count > 0 {
				if status.Entries[0].TaskID != "t1" {
					t.Fatalf("expected queued task_id t1, got %s", status.Entries[0].TaskID)
				}
				if status.Entries[0].QueuedBy != messagequeue.QueuedByWorkflow {
					t.Fatalf("expected queued_by workflow, got %s", status.Entries[0].QueuedBy)
				}
				meta := status.Entries[0].Metadata
				if got := meta["workflow_message"]; got != true {
					t.Fatalf("workflow_message = %v, want true", got)
				}
				if got := meta["workflow_step_id"]; got != "step2" {
					t.Fatalf("workflow_step_id = %v, want step2", got)
				}
				if got := meta["workflow_step_name"]; got != "In Progress" {
					t.Fatalf("workflow_step_name = %v, want In Progress", got)
				}
				if got := meta["workflow_step_color"]; got != "bg-emerald-500" {
					t.Fatalf("workflow_step_color = %v, want bg-emerald-500", got)
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
		queued, err := svc.queueAutoStartPromptIfRunning(ctx, "t1", session, "prompt", false, nil, workflowMessageOrigin{}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if queued {
			t.Error("expected pre-check NOT to queue when session state is CREATED")
		}
	})

	// Regression: an in-memory session pointer can read RUNNING/STARTING even
	// when the agent's turn is actually done (the manual-move goroutine loaded
	// the row before agent.ready fired, or the previous turn's bookkeeping
	// hasn't propagated). Without flipStaleRunningToWaiting, processOnEnter
	// would queue the auto-start prompt against this stale state and nothing
	// would drain it — the symptom users observe as "QUEUED 1 of N" stuck in
	// the chat after a step move.
	t.Run("flipStaleRunningToWaiting flips RUNNING session with no active turn", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1") // seeds session.State = RUNNING

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// Intentionally do NOT seed svc.activeTurns — this simulates the stale
		// state where session.State claims RUNNING but no turn is in flight.

		session, _ := repo.GetTaskSession(ctx, "s1")
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("precondition: session should be RUNNING, got %q", session.State)
		}

		flipped := svc.flipStaleRunningToWaiting(ctx, "t1", session, false)
		if !flipped {
			t.Fatal("expected flipStaleRunningToWaiting to flip stale RUNNING session")
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("in-memory state: want WAITING_FOR_INPUT, got %q", session.State)
		}
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("DB state: want WAITING_FOR_INPUT, got %q", updated.State)
		}
	})

	// Regression for #1036: when activeTurns has an entry, the session is
	// genuinely mid-turn; agent.ready will drain the queue. The helper must
	// leave state alone so the queue path stays the right answer.
	t.Run("flipStaleRunningToWaiting leaves session running when active turn registered", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.activeTurns.Store("s1", "turn-active")

		session, _ := repo.GetTaskSession(ctx, "s1")
		flipped := svc.flipStaleRunningToWaiting(ctx, "t1", session, false)
		if flipped {
			t.Fatal("expected flipStaleRunningToWaiting to no-op when an active turn is registered")
		}
		if session.State != models.TaskSessionStateRunning {
			t.Errorf("in-memory state: want RUNNING preserved, got %q", session.State)
		}
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateRunning {
			t.Errorf("DB state: want RUNNING preserved, got %q", updated.State)
		}
	})

	t.Run("flipStaleRunningToWaiting flips STARTING session with no active turn", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateStarting
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// No activeTurns entry — STARTING with no registered turn should flip.
		flipped := svc.flipStaleRunningToWaiting(ctx, "t1", session, false)
		if !flipped {
			t.Fatal("expected STARTING session with no active turn to be flipped to WAITING_FOR_INPUT")
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("in-memory state: want WAITING_FOR_INPUT, got %q", session.State)
		}
		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("DB state: want WAITING_FOR_INPUT, got %q", updated.State)
		}
	})

	t.Run("flipStaleRunningToWaiting no-ops for passthrough sessions", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		session, _ := repo.GetTaskSession(ctx, "s1")

		flipped := svc.flipStaleRunningToWaiting(ctx, "t1", session, true)
		if flipped {
			t.Fatal("expected flipStaleRunningToWaiting to no-op for passthrough sessions")
		}
		if session.State != models.TaskSessionStateRunning {
			t.Errorf("passthrough session state must be left alone, got %q", session.State)
		}
	})

	t.Run("flipStaleRunningToWaiting no-ops for CREATED sessions", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateCreated
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		flipped := svc.flipStaleRunningToWaiting(ctx, "t1", session, false)
		if flipped {
			t.Fatal("expected flipStaleRunningToWaiting to no-op for CREATED sessions")
		}
		if session.State != models.TaskSessionStateCreated {
			t.Errorf("CREATED session state must be left alone, got %q", session.State)
		}
	})

	t.Run("flipStaleRunningToWaiting no-ops while session reset is in progress", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// resetAgentContext owns the state machine during reset; the helper
		// must not race with it.
		svc.setSessionResetInProgress("s1", true)
		t.Cleanup(func() { svc.setSessionResetInProgress("s1", false) })

		session, _ := repo.GetTaskSession(ctx, "s1")
		flipped := svc.flipStaleRunningToWaiting(ctx, "t1", session, false)
		if flipped {
			t.Fatal("expected flipStaleRunningToWaiting to no-op while reset is in progress")
		}
		if session.State != models.TaskSessionStateRunning {
			t.Errorf("session state must be left alone during reset, got %q", session.State)
		}
	})

	t.Run("handles missing from-step gracefully", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
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

			synctest.Wait()
		})
	})

	t.Run("handles missing to-step gracefully", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
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

			synctest.Wait()
		})
	})

	t.Run("reset_agent_context processed on enter", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			// Set agent execution ID on the session
			session, _ := repo.GetTaskSession(ctx, "s1")
			session.AgentExecutionID = "exec-123"
			seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-123")
			_ = repo.UpdateTaskSession(ctx, session)
			_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"acp_session_id": "old-acp"})

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

			agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentMgr)
			svc.handleTaskMovedWithSession(ctx, watcher.TaskMovedEventData{
				TaskID:          "t1",
				SessionID:       "s1",
				FromStepID:      "step1",
				ToStepID:        "step2",
				TaskDescription: "test task",
			})

			synctest.Wait()

			agentMgr.mu.Lock()
			restartCalls := agentMgr.restartProcessCalls
			agentMgr.mu.Unlock()
			if len(restartCalls) != 1 {
				t.Fatalf("expected 1 RestartAgentProcess call, got %d", len(restartCalls))
			}
			if restartCalls[0] != "exec-123" {
				t.Errorf("expected RestartAgentProcess called with 'exec-123', got %q", restartCalls[0])
			}

			// Verify acp_session_id was cleared
			updated, _ := repo.GetTaskSession(ctx, "s1")
			if updated.Metadata != nil {
				if acp, _ := updated.Metadata["acp_session_id"].(string); acp != "" {
					t.Error("expected acp_session_id to be cleared from session metadata")
				}
			}
		})
	})
}

func TestProcessStepExitAndEnter(t *testing.T) {
	ctx := context.Background()

	t.Run("runs on_exit then on_enter", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set plan_mode on session — on_exit should clear it
		session, _ := repo.GetTaskSession(ctx, "s1")
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"plan_mode": true})

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
			if pm, _ := updated.Metadata["plan_mode"].(bool); pm {
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
