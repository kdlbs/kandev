package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestHandleTaskCreated covers WO-36: a task created directly onto a step
// whose on_enter carries auto_start_agent (e.g. a materialized heavy
// routine run landing on the Routine workflow's start step) never got its
// on_enter evaluated, because every other autoStartTaskForStep caller
// represents a transition INTO a step (task.moved, task.queue_promoted,
// dependency resolution) — creation was never wired in.
func TestHandleTaskCreated(t *testing.T) {
	ctx := context.Background()

	t.Run("launches a session for a task created directly on an auto-start step", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

		// Mirrors what CreateOfficeTaskInWorkflow now stamps for a heavy
		// routine run: the routine's assignee carried as a task-level
		// fallback, since the Routine workflow's start step pins no agent.
		metadata := map[string]interface{}{
			models.MetaKeyAgentProfileID: "routine-assignee",
		}
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:             "t1",
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step1",
			Title:          "Routine run",
			Description:    "prompt",
			State:          v1.TaskStateCreated,
			Metadata:       metadata,
			CreatedAt:      now,
			UpdatedAt:      now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
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
				agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
				launched <- agentProfileID
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
			},
		}
		svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t1"})

		select {
		case got := <-launched:
			if got != "routine-assignee" {
				t.Fatalf("AgentProfileID = %q, want routine-assignee", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for auto-start launch on task creation")
		}
	})

	t.Run("skips when the task carries deferred-launch metadata", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

		// A REST/MCP/WS create request with start_agent (or prepare_session)
		// stamps MetaKeyDeferredLaunch unconditionally. That request handler
		// either already launched the session synchronously, or the task is
		// queued/blocked and a later transition/dependency-resolution event
		// owns consuming the deferred-launch record. Either way, task.created
		// must not also race a launch through the auto-start chokepoint.
		metadata := map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent":           "start",
				"agent_profile_id": "explicit-profile",
			},
		}
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:             "t2",
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step1",
			Title:          "Explicit start",
			Description:    "prompt",
			State:          v1.TaskStateCreated,
			Metadata:       metadata,
			CreatedAt:      now,
			UpdatedAt:      now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Start", Position: 0,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))

		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t2"})
	})

	t.Run("skips when the task cannot be found", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		// Should not panic — task not found is logged and returns.
		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "nonexistent"})
	})
}
