package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestStopDynamicRelaunchPredecessor_EmptyExecutionIDCountsAsStopped(t *testing.T) {
	repo := setupTestRepo(t)
	agentManager := &mockAgentManager{}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)

	if !svc.stopDynamicRelaunchPredecessor(context.Background(), "") {
		t.Fatal("an orphaned route with no recorded execution must count as stopped")
	}
	if len(agentManager.stopAgentWithReasonArgs) != 0 {
		t.Fatalf("stop calls = %d, want none for an absent predecessor", len(agentManager.stopAgentWithReasonArgs))
	}
}

func TestDynamicRelaunchHasResumeSource(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-orphan-route-source"
		sessionID = "session-orphan-route-source"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})

	if svc.dynamicRelaunchHasResumeSource(ctx, sessionID) {
		t.Fatal("a session without an executors_running row has no cold-resume source")
	}
	seedExecutorRunning(t, repo, sessionID, taskID, "execution-orphan-route-source")
	if !svc.dynamicRelaunchHasResumeSource(ctx, sessionID) {
		t.Fatal("a session with an executors_running row keeps the resume path")
	}
}

func TestPrepareDynamicRelaunchAfterFailure_ManualOrphanedRouteStartsFresh(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-orphan-route-manual"
		sessionID = "session-orphan-route-manual"
	)
	repo := setupTestRepo(t)
	// The restart orphan sweep parks the session awaiting input with its
	// route action_required and no executors_running row.
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentManager := &mockAgentManager{}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the orphaned route"})

	_, _, prompt, launchMode, ok := svc.prepareDynamicRelaunchAfterFailure(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID,
	}, launchOriginManual)
	if !ok {
		t.Fatal("a manual relaunch of an orphaned route without an execution must be prepared")
	}
	if launchMode != dynamicRelaunchLaunchModeCreated {
		t.Fatalf("launch mode = %v, want a fresh created-session launch without a resume source", launchMode)
	}
	if prompt.text != "retry the orphaned route" {
		t.Fatalf("relaunch prompt = %q, want the latest user prompt", prompt.text)
	}
	if len(agentManager.stopAgentWithReasonArgs) != 0 {
		t.Fatalf("stop calls = %d, want none for an absent predecessor", len(agentManager.stopAgentWithReasonArgs))
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if session.State != models.TaskSessionStateCreated {
		t.Fatalf("session state = %q, want CREATED for a fresh launch", session.State)
	}
}
