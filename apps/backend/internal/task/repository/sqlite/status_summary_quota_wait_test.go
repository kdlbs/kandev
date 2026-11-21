package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
)

func TestLoadTaskStatusSummaryQuotaWaitsFiltersCurrentRoutesAndOrdersSessions(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const workspaceID = "ws-quota-waits"
	seedWorkspace(t, repo, workspaceID)
	for _, taskID := range []string{
		"task-order",
		"task-stale-generation",
		"task-stale-profile",
		"task-terminal",
		"task-chunked",
	} {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
	}

	startedAt := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	addSession := func(taskID, sessionID string, state models.TaskSessionState, profileID string, generation int64, primary bool, started time.Time) {
		t.Helper()
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: taskID, State: state,
			AgentProfileID: profileID, RouteGeneration: generation,
			IsPrimary: primary, StartedAt: started,
		}); err != nil {
			t.Fatalf("create session %s: %v", sessionID, err)
		}
	}
	addRoute := func(sessionID, profileID string, generation int64, status string, deadline time.Time) {
		t.Helper()
		policyJSON, err := json.Marshal(dynamicruntime.PolicyState{
			FailureCode: routingerr.CodeQuotaLimited,
			Deadline:    &deadline,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.SaveRouteState(ctx, dynamicruntime.RouteState{
			SessionID: sessionID, LogicalProfileID: profileID, Generation: generation,
			Status: status, PolicyStateJSON: string(policyJSON), UpdatedAt: startedAt,
		}); err != nil {
			t.Fatalf("save route %s: %v", sessionID, err)
		}
	}
	deadline := time.Date(2026, 11, 9, 12, 30, 0, 0, time.UTC)

	// The primary session is older than the secondary session. Primary ownership
	// still wins over recency when both routes hold a quota wait.
	addSession("task-order", "session-secondary", models.TaskSessionStateWaitingForInput, "profile-order", 4, false, startedAt.Add(time.Hour))
	addRoute("session-secondary", "profile-order", 4, "waiting_for_reset", deadline.Add(time.Hour))
	addSession("task-order", "session-primary", models.TaskSessionStateWaitingForInput, "profile-order", 4, true, startedAt)
	addRoute("session-primary", "profile-order", 4, "retry_wait", deadline)

	addSession("task-stale-generation", "session-stale-generation", models.TaskSessionStateWaitingForInput, "profile-generation", 5, true, startedAt)
	addRoute("session-stale-generation", "profile-generation", 4, "retry_wait", deadline)
	addSession("task-stale-profile", "session-stale-profile", models.TaskSessionStateWaitingForInput, "profile-current", 5, true, startedAt)
	addRoute("session-stale-profile", "profile-obsolete", 5, "retry_wait", deadline)
	addSession("task-terminal", "session-terminal", models.TaskSessionStateCompleted, "profile-terminal", 1, true, startedAt)
	addRoute("session-terminal", "profile-terminal", 1, "retry_wait", deadline)
	addSession("task-chunked", "session-chunked", models.TaskSessionStateWaitingForInput, "profile-chunked", 2, true, startedAt)
	addRoute("session-chunked", "profile-chunked", 2, "retry_wait", deadline)

	taskIDs := []string{"task-order", "task-stale-generation", "task-stale-profile", "task-terminal"}
	for i := 0; i < 2*sqliteMaxHostParams; i++ {
		taskIDs = append(taskIDs, fmt.Sprintf("padding-task-%d", i))
	}
	// Put a real result in the third chunk to prove that later chunks are read.
	taskIDs = append(taskIDs, "task-chunked")

	got, err := repo.LoadTaskStatusSummaryQuotaWaits(ctx, taskIDs)
	if err != nil {
		t.Fatalf("LoadTaskStatusSummaryQuotaWaits: %v", err)
	}
	if wait := got["task-order"]; wait == nil || wait.SessionID != "session-primary" || !wait.Deadline.Equal(deadline) {
		t.Fatalf("primary wait = %+v, want primary session and deadline %s", wait, deadline)
	}
	for _, taskID := range []string{"task-stale-generation", "task-stale-profile", "task-terminal"} {
		if wait := got[taskID]; wait != nil {
			t.Errorf("wait for %s = %+v, want no wait", taskID, wait)
		}
	}
	if wait := got["task-chunked"]; wait == nil || wait.SessionID != "session-chunked" {
		t.Fatalf("third-chunk wait = %+v, want session-chunked", wait)
	}
}
