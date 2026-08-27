package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// assertSessionLauncherNotCalled waits a bounded window for launchAutoStartTask's
// goroutine to have fired LaunchSession, then fails if it did. auto-start
// dispatch is asynchronous (handlers.go's launchAutoStartTask spawns a
// goroutine), so simply checking immediately after handleCreateTask returns
// would race a real bug rather than catch it.
func assertSessionLauncherNotCalled(t *testing.T, launcher *mockSessionLauncher) {
	t.Helper()
	select {
	case <-launcher.called:
		t.Fatal("LaunchSession must not be called for a Found outcome, even with start_agent:true")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestHandleCreateTask_ExternalIDGoldenPath covers the golden path: a create
// carrying a fresh external_id reports deduplicated:false, creation_complete
// :true, and echoes external_id in the tool result.
func TestHandleCreateTask_ExternalIDGoldenPath(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	resp, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      workflows[0].ID,
		"title":            "Task",
		"description":      "Do the thing",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
		"external_id":      "ext-1",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	if resp.Type == ws.MessageTypeError {
		t.Fatalf("create task returned error: %s", string(resp.Payload))
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &result))
	require.Equal(t, "ext-1", result["external_id"])
	require.Equal(t, false, result["deduplicated"])
	require.Equal(t, true, result["creation_complete"])
}

// TestHandleCreateTask_FoundSettledIsDataLossGuarded is the spec's headline
// scenario: a retry that carries an external_id already held by a settled
// task, but with a DIFFERENT repository_url that would otherwise resolve to
// a remote contribution, must return the existing task unmodified — not run
// contribution association, which indexes against the returned task's
// repositories and rolls back (deletes!) on any mismatch. If the guard is
// missing or broken, this test observes the task actually get deleted.
func TestHandleCreateTask_FoundSettledIsDataLossGuarded(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	remote := &recordingRemoteContributionService{resolution: testRemoteContributionResolution()}
	h.SetRemoteContributionService(remote)

	first, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      workflows[0].ID,
		"title":            "Original",
		"description":      "Do the thing",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
		"external_id":      "ext-1",
	}))
	require.NoError(t, err)
	if first.Type == ws.MessageTypeError {
		t.Fatalf("first create returned error: %s", string(first.Payload))
	}
	var firstResult map[string]interface{}
	require.NoError(t, json.Unmarshal(first.Payload, &firstResult))
	firstID := firstResult["id"].(string)

	retry, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      workflows[0].ID,
		"title":            "Changed title",
		"description":      "Different prompt",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
		"external_id":      "ext-1",
		"repositories": []map[string]interface{}{{
			"github_url":  "https://github.com/acme/widget/pull/7",
			"base_branch": "main",
		}},
	}))
	require.NoError(t, err)
	if retry.Type == ws.MessageTypeError {
		t.Fatalf("retry returned error: %s", string(retry.Payload))
	}

	var retryResult map[string]interface{}
	require.NoError(t, json.Unmarshal(retry.Payload, &retryResult))
	require.Equal(t, firstID, retryResult["id"], "the existing task must be returned")
	require.Equal(t, "Original", retryResult["title"], "the existing task must be returned unchanged")
	require.Equal(t, true, retryResult["deduplicated"])
	require.Equal(t, true, retryResult["creation_complete"])

	// remote.associateURL is set by Resolve, which runs unconditionally on
	// the raw request before the outcome is known — that's pre-existing,
	// correct behavior. The guard under test is that Associate (the step
	// that indexes against the RETURNED task's repositories and rolls back
	// on any mismatch) never runs on a Found outcome.
	require.Empty(t, remote.taskID, "no remote contribution association should be attempted on a Found outcome")
	require.Empty(t, remote.repositoryID, "no remote contribution association should be attempted on a Found outcome")

	survivor, err := svc.GetTask(ctx, firstID)
	require.NoError(t, err)
	require.NotNil(t, survivor, "the task must still exist — this is the data-loss guard")
}

// TestHandleCreateTask_FoundUnsettledSkipsAutoStart covers the diagnostic
// tuple for an in-flight create: deduplicated:true + creation_complete:false,
// and confirms no session is auto-started for it even when start_agent:true.
//
// A nil sessionLauncher (as used elsewhere in this file) would make an
// auto-start assertion vacuous: launchAutoStartTask no-ops on a nil launcher
// regardless of whether the Found-outcome guard even ran. Wiring a real
// mockSessionLauncher makes "no session was launched" an actual proof rather
// than an artifact of the test harness.
func TestHandleCreateTask_FoundUnsettledSkipsAutoStart(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	launcher := newMockSessionLauncher()
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, launcher, nil, testLogger(t))

	// Seed a task holding ext-1 directly, unsettled, representing a create
	// whose required synchronous work has not finished yet.
	inflight := &models.Task{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflows[0].ID,
		Title:       "In flight",
		ExternalID:  "ext-1",
	}
	require.NoError(t, repo.CreateTask(ctx, inflight))

	resp, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      workflows[0].ID,
		"title":            "Task",
		"description":      "Do the thing",
		"agent_profile_id": "profile-1",
		"start_agent":      true,
		"external_id":      "ext-1",
	}))
	require.NoError(t, err)
	if resp.Type == ws.MessageTypeError {
		t.Fatalf("create returned error: %s", string(resp.Payload))
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &result))
	require.Equal(t, inflight.ID, result["id"])
	require.Equal(t, true, result["deduplicated"])
	require.Equal(t, false, result["creation_complete"])

	assertSessionLauncherNotCalled(t, launcher)

	// The response tuple alone doesn't prove nothing was launched — query the
	// real database directly for the downstream effect start_agent:true would
	// have produced had the guard been missing.
	sessions, err := repo.ListTaskSessions(ctx, inflight.ID)
	require.NoError(t, err)
	require.Empty(t, sessions, "no session row should exist for an unsettled task even with start_agent:true")
}

// TestHandleCreateTask_FoundSettledSkipsAutoStart is the settled-outcome
// twin of TestHandleCreateTask_FoundUnsettledSkipsAutoStart: the spec's very
// first MCP scenario is a retry against a SETTLED task with start_agent:true,
// and requires that no new session gets started. The Found-outcome guard in
// handleCreateTask (handlers.go) is a single check that covers both settled
// and unsettled sub-cases identically, but until now only the unsettled
// sub-case had a dedicated auto-start assertion.
func TestHandleCreateTask_FoundSettledSkipsAutoStart(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	launcher := newMockSessionLauncher()
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, launcher, nil, testLogger(t))

	first, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      workflows[0].ID,
		"title":            "Original",
		"description":      "Do the thing",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
		"external_id":      "ext-1",
	}))
	require.NoError(t, err)
	if first.Type == ws.MessageTypeError {
		t.Fatalf("first create returned error: %s", string(first.Payload))
	}
	var firstResult map[string]interface{}
	require.NoError(t, json.Unmarshal(first.Payload, &firstResult))
	firstID := firstResult["id"].(string)
	require.Equal(t, true, firstResult["creation_complete"], "the seeding create must have settled")

	assertSessionLauncherNotCalled(t, launcher) // sanity: the seeding create itself didn't launch

	retry, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      workflows[0].ID,
		"title":            "Task",
		"description":      "Do the thing",
		"agent_profile_id": "profile-1",
		"start_agent":      true,
		"external_id":      "ext-1",
	}))
	require.NoError(t, err)
	if retry.Type == ws.MessageTypeError {
		t.Fatalf("retry returned error: %s", string(retry.Payload))
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(retry.Payload, &result))
	require.Equal(t, firstID, result["id"])
	require.Equal(t, true, result["deduplicated"])
	require.Equal(t, true, result["creation_complete"])

	assertSessionLauncherNotCalled(t, launcher)

	sessions, err := repo.ListTaskSessions(ctx, firstID)
	require.NoError(t, err)
	require.Empty(t, sessions, "a settled Found outcome must not start a new session even with start_agent:true")
}
