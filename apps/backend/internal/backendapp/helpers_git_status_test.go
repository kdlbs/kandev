package backendapp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/task/models"
	sqlitetaskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type gitStatusEnvironmentFixture struct {
	repo      *sqlitetaskrepo.Repository
	env       *models.TaskEnvironment
	requested *models.TaskSession
}

func newGitStatusEnvironmentFixture(t *testing.T) gitStatusEnvironmentFixture {
	t.Helper()
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	const (
		taskID         = "git-status-task"
		environmentID  = "git-status-environment"
		requestedID    = "git-status-requested"
		canonicalID    = "git-status-canonical"
		canonicalPath  = "/tasks/git-status/canonical"
		mismatchedPath = "/tasks/git-status/mismatched"
	)
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	if err := harness.taskRepo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	env := &models.TaskEnvironment{
		ID:            environmentID,
		TaskID:        taskID,
		ExecutorType:  string(models.ExecutorTypeLocal),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: canonicalPath,
	}
	if err := harness.taskRepo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	for _, session := range []*models.TaskSession{
		{
			ID:                requestedID,
			TaskID:            taskID,
			TaskEnvironmentID: environmentID,
			WorkspacePath:     mismatchedPath,
			State:             models.TaskSessionStateWaitingForInput,
			StartedAt:         now,
			UpdatedAt:         now,
		},
		{
			ID:                canonicalID,
			TaskID:            taskID,
			TaskEnvironmentID: environmentID,
			WorkspacePath:     canonicalPath,
			State:             models.TaskSessionStateWaitingForInput,
			StartedAt:         now.Add(time.Minute),
			UpdatedAt:         now.Add(time.Minute),
		},
	} {
		if err := harness.taskRepo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	if err := harness.taskRepo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:           "git-status-mismatched-snapshot",
		SessionID:    requestedID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Files: map[string]interface{}{
			"mismatched.go": map[string]interface{}{"status": "modified"},
		},
		Metadata: map[string]interface{}{
			"timestamp": "2026-08-30T12:01:00Z",
			"modified":  []string{"mismatched.go"},
		},
		CreatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(mismatched): %v", err)
	}
	if err := harness.taskRepo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:           "git-status-canonical-snapshot",
		SessionID:    canonicalID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Files: map[string]interface{}{
			"canonical.go": map[string]interface{}{"status": "modified"},
		},
		Metadata: map[string]interface{}{
			"timestamp": "2026-08-30T12:03:00Z",
			"modified":  []string{"canonical.go"},
		},
		CreatedAt: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(canonical): %v", err)
	}

	requested, err := harness.taskRepo.GetTaskSession(ctx, requestedID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	return gitStatusEnvironmentFixture{repo: harness.taskRepo, env: env, requested: requested}
}

func TestAppendLiveGitStatusMessageSelectsCanonicalEnvironmentSnapshot(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	msgs := appendLiveGitStatusMessage(
		context.Background(), fixture.repo, nil, fixture.requested.ID, fixture.requested, nil, newTestLogger(),
	)
	if len(msgs) != 1 {
		t.Fatalf("expected one git status message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	if got := payload["session_id"]; got != fixture.requested.ID {
		t.Fatalf("event session_id = %v, want requested session %q", got, fixture.requested.ID)
	}
	status, ok := payload["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status payload = %#v, want object", payload["status"])
	}
	files, ok := status["files"].(map[string]interface{})
	if !ok {
		t.Fatalf("status files = %#v, want object", status["files"])
	}
	if _, ok := files["canonical.go"]; !ok {
		t.Fatalf("canonical snapshot was not selected: files = %#v", files)
	}
	if _, ok := files["mismatched.go"]; ok {
		t.Fatalf("mismatched snapshot was selected: files = %#v", files)
	}
}

func TestAppendLiveGitStatusMessageSelectsCanonicalSiblingLiveStatus(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	log := newTestLogger()
	agentClient, closeServer := newGitStatusClient(t, log, client.GitStatusResult{
		Success:   true,
		Modified:  []string{"canonical-live.go"},
		Timestamp: "2026-08-30T12:04:00Z",
		Files:     map[string]interface{}{"canonical-live.go": map[string]interface{}{"status": "modified"}},
	})
	defer closeServer()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID:        "git-status-canonical-execution",
		TaskID:    fixture.requested.TaskID,
		SessionID: "git-status-canonical",
		// Recovered executions can predate the environment identity on the
		// in-memory lifecycle record. The canonical workspace path remains a
		// sufficient binding after the session source has been verified.
		TaskEnvironmentID: "",
		WorkspacePath:     fixture.env.WorkspacePath,
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	execution, ok := mgr.GetExecutionBySessionID("git-status-canonical")
	if !ok {
		t.Fatal("canonical execution was not registered")
	}
	execution.SetAgentCtlClientForTesting(agentClient)

	msgs := appendLiveGitStatusMessage(
		context.Background(), fixture.repo, mgr, fixture.requested.ID, fixture.requested, nil, log,
	)
	if len(msgs) != 1 {
		t.Fatalf("expected one git status message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	if got := payload["session_id"]; got != fixture.requested.ID {
		t.Fatalf("event session_id = %v, want requested session %q", got, fixture.requested.ID)
	}
	status, ok := payload["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status payload = %#v, want object", payload["status"])
	}
	files, ok := status["files"].(map[string]interface{})
	if !ok {
		t.Fatalf("status files = %#v, want object", status["files"])
	}
	if _, ok := files["canonical-live.go"]; !ok {
		t.Fatalf("canonical sibling live status was not selected: files = %#v", files)
	}
}

func TestAppendLiveGitStatusMessageFallsBackWhenRequestedSessionIsNotCanonical(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	log := newTestLogger()
	agentClient, closeServer := newGitStatusClient(t, log, client.GitStatusResult{
		Success:   true,
		Modified:  []string{"mismatched-live.go"},
		Timestamp: "2026-08-30T12:04:00Z",
		Files:     map[string]interface{}{"mismatched-live.go": map[string]interface{}{"status": "modified"}},
	})
	defer closeServer()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID:                "git-status-mismatched-execution",
		TaskID:            fixture.requested.TaskID,
		SessionID:         fixture.requested.ID,
		TaskEnvironmentID: fixture.env.ID,
		WorkspacePath:     "/tasks/git-status/mismatched-runtime",
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	execution, ok := mgr.GetExecutionBySessionID(fixture.requested.ID)
	if !ok {
		t.Fatal("mismatched execution was not registered")
	}
	execution.SetAgentCtlClientForTesting(agentClient)

	msgs := appendLiveGitStatusMessage(
		context.Background(), fixture.repo, mgr, fixture.requested.ID, fixture.requested, nil, log,
	)
	if len(msgs) != 1 {
		t.Fatalf("expected one git status message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	status, ok := payload["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status payload = %#v, want object", payload["status"])
	}
	files, ok := status["files"].(map[string]interface{})
	if !ok {
		t.Fatalf("status files = %#v, want object", status["files"])
	}
	if _, ok := files["canonical.go"]; !ok {
		t.Fatalf("mismatched live execution bypassed canonical snapshot: files = %#v", files)
	}
	if _, ok := files["mismatched-live.go"]; ok {
		t.Fatalf("mismatched live status was selected: files = %#v", files)
	}
}

func TestAppendLiveGitStatusMessageRejectsMismatchedLiveExecutionWorkspace(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	ctx := context.Background()
	if _, err := fixture.repo.DB().ExecContext(ctx, `
		UPDATE task_sessions SET workspace_path = ? WHERE id = ?
	`, fixture.env.WorkspacePath, fixture.requested.ID); err != nil {
		t.Fatalf("update requested workspace path: %v", err)
	}
	requested, err := fixture.repo.GetTaskSession(ctx, fixture.requested.ID)
	if err != nil {
		t.Fatalf("reload requested session: %v", err)
	}
	if err := fixture.repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:           "git-status-requested-fallback-snapshot",
		SessionID:    requested.ID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Files:        map[string]interface{}{"fallback.go": map[string]interface{}{"status": "modified"}},
		Metadata: map[string]interface{}{
			"timestamp": "2026-08-30T12:06:00Z",
			"modified":  []string{"fallback.go"},
		},
		CreatedAt: time.Date(2026, 8, 30, 12, 6, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create fallback snapshot: %v", err)
	}

	log := newTestLogger()
	agentClient, closeServer := newGitStatusClient(t, log, client.GitStatusResult{
		Success:   true,
		Modified:  []string{"mismatched-live.go"},
		Timestamp: "2026-08-30T12:07:00Z",
		Files:     map[string]interface{}{"mismatched-live.go": map[string]interface{}{"status": "modified"}},
	})
	defer closeServer()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID:                "git-status-requested-mismatched-runtime",
		TaskID:            requested.TaskID,
		SessionID:         requested.ID,
		TaskEnvironmentID: fixture.env.ID,
		WorkspacePath:     "/tasks/git-status/mismatched-runtime",
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	execution, ok := mgr.GetExecutionBySessionID(requested.ID)
	if !ok {
		t.Fatal("mismatched execution was not registered")
	}
	execution.SetAgentCtlClientForTesting(agentClient)

	msgs := appendLiveGitStatusMessage(ctx, fixture.repo, mgr, requested.ID, requested, nil, log)
	if len(msgs) != 1 {
		t.Fatalf("expected one git status message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	status, ok := payload["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status payload = %#v, want object", payload["status"])
	}
	files, ok := status["files"].(map[string]interface{})
	if !ok {
		t.Fatalf("status files = %#v, want object", status["files"])
	}
	if _, ok := files["fallback.go"]; !ok {
		t.Fatalf("mismatched runtime did not fall back to the eligible snapshot: files = %#v", files)
	}
	if _, ok := files["mismatched-live.go"]; ok {
		t.Fatalf("mismatched runtime status was selected: files = %#v", files)
	}
}

func TestAppendLiveGitStatusMessageDoesNotFallbackAfterLiveQueryFailure(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	log := newTestLogger()
	agentClient, closeServer := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "agentctl unavailable", http.StatusServiceUnavailable)
	}))
	defer closeServer()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID:                "git-status-live-failure-execution",
		TaskID:            fixture.requested.TaskID,
		SessionID:         "git-status-canonical",
		TaskEnvironmentID: fixture.env.ID,
		WorkspacePath:     fixture.env.WorkspacePath,
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	execution, ok := mgr.GetExecutionBySessionID("git-status-canonical")
	if !ok {
		t.Fatal("canonical execution was not registered")
	}
	execution.SetAgentCtlClientForTesting(agentClient)

	msgs := appendLiveGitStatusMessage(
		context.Background(), fixture.repo, mgr, fixture.requested.ID, fixture.requested, nil, log,
	)
	if len(msgs) != 0 {
		t.Fatalf("got %d status messages after live query failure, want no persisted fallback", len(msgs))
	}
}

func TestAppendLiveGitStatusMessageDropsResultFromReplacedExecution(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	log := newTestLogger()
	started := make(chan struct{})
	release := make(chan struct{})
	agentClient, closeServer := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/git/status/multi" {
			http.NotFound(w, r)
			return
		}
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{
			Success: true,
			Repos: []client.PerRepoGitStatus{{Status: client.GitStatusResult{
				Success: true, StatusState: "ready", FilesComplete: true, DetailState: "ready",
				Modified: []string{"retired.txt"}, Files: map[string]interface{}{"retired.txt": map[string]interface{}{"status": "modified"}},
			}}},
		})
	}))
	defer closeServer()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	oldExecution := &lifecycle.AgentExecution{
		ID: "git-status-live-old", TaskID: fixture.requested.TaskID, SessionID: "git-status-canonical",
		TaskEnvironmentID: fixture.env.ID, WorkspacePath: fixture.env.WorkspacePath,
	}
	if err := mgr.ExecutionStoreForTesting().Add(oldExecution); err != nil {
		t.Fatalf("add old source execution: %v", err)
	}
	oldExecution.SetAgentCtlClientForTesting(agentClient)

	result := make(chan int, 1)
	go func() {
		messages := appendLiveGitStatusMessage(context.Background(), fixture.repo, mgr, fixture.requested.ID, fixture.requested, nil, log)
		result <- len(messages)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial subscription did not enter the live status request")
	}
	mgr.ExecutionStoreForTesting().Remove(oldExecution.ID)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID: "git-status-live-new", TaskID: fixture.requested.TaskID, SessionID: "git-status-canonical",
		TaskEnvironmentID: fixture.env.ID, WorkspacePath: fixture.env.WorkspacePath,
	}); err != nil {
		t.Fatalf("add replacement execution: %v", err)
	}
	close(release)
	select {
	case count := <-result:
		if count != 0 {
			t.Fatalf("initial subscription forwarded %d stale status messages, want zero", count)
		}
	case <-time.After(time.Second):
		t.Fatal("initial subscription did not finish after source replacement")
	}
}

func TestAppendLiveGitStatusMessageRejectsUnrecordedWorkspace(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	ctx := context.Background()
	if _, err := fixture.repo.DB().ExecContext(ctx, `
		UPDATE task_sessions SET workspace_path = '' WHERE id = ?
	`, fixture.requested.ID); err != nil {
		t.Fatalf("clear requested workspace path: %v", err)
	}
	requested, err := fixture.repo.GetTaskSession(ctx, fixture.requested.ID)
	if err != nil {
		t.Fatalf("reload requested session: %v", err)
	}

	msgs := appendLiveGitStatusMessage(ctx, fixture.repo, nil, requested.ID, requested, nil, newTestLogger())
	if len(msgs) != 1 {
		t.Fatalf("expected one git status message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	status, ok := payload["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status payload = %#v, want object", payload["status"])
	}
	files, ok := status["files"].(map[string]interface{})
	if !ok {
		t.Fatalf("status files = %#v, want object", status["files"])
	}
	if _, ok := files["canonical.go"]; !ok {
		t.Fatalf("canonical sibling snapshot was not selected: files = %#v", files)
	}
	if _, ok := files["mismatched.go"]; ok {
		t.Fatalf("unrecorded session snapshot was selected: files = %#v", files)
	}
}

func TestAppendLiveGitStatusMessageSelectsSharedEnvironmentAcrossTasks(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	ctx := context.Background()
	const ownerTaskID = "git-status-owner-task"
	if err := fixture.repo.CreateTask(ctx, &models.Task{ID: ownerTaskID, Title: ownerTaskID}); err != nil {
		t.Fatalf("create owner task: %v", err)
	}
	if _, err := fixture.repo.DB().ExecContext(ctx, `
		UPDATE task_environments SET task_id = ? WHERE id = ?
	`, ownerTaskID, fixture.env.ID); err != nil {
		t.Fatalf("move environment owner: %v", err)
	}
	ownerSessionID := "git-status-owner-session"
	ownerSessionTime := time.Date(2026, 8, 30, 12, 7, 0, 0, time.UTC)
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                ownerSessionID,
		TaskID:            ownerTaskID,
		TaskEnvironmentID: fixture.env.ID,
		WorkspacePath:     fixture.env.WorkspacePath,
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         ownerSessionTime,
		UpdatedAt:         ownerSessionTime,
	}); err != nil {
		t.Fatalf("create owner session: %v", err)
	}
	if err := fixture.repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:           "git-status-owner-snapshot",
		SessionID:    ownerSessionID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Files:        map[string]interface{}{"owner.go": map[string]interface{}{"status": "modified"}},
		Metadata: map[string]interface{}{
			"timestamp": "2026-08-30T12:10:00Z",
			"modified":  []string{"owner.go"},
		},
		CreatedAt: time.Date(2026, 8, 30, 12, 10, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create owner snapshot: %v", err)
	}

	msgs := appendLiveGitStatusMessage(ctx, fixture.repo, nil, fixture.requested.ID, fixture.requested, nil, newTestLogger())
	if len(msgs) != 1 {
		t.Fatalf("expected one git status message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	status, ok := payload["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status payload = %#v, want object", payload["status"])
	}
	files, ok := status["files"].(map[string]interface{})
	if !ok {
		t.Fatalf("status files = %#v, want object", status["files"])
	}
	if _, ok := files["owner.go"]; !ok {
		t.Fatalf("shared environment owner snapshot was not selected: files = %#v", files)
	}
	if _, ok := files["canonical.go"]; ok {
		t.Fatalf("same-task snapshot won over newer shared-environment snapshot: files = %#v", files)
	}
}

func TestAppendLiveGitStatusMessageSelectsNewestSnapshotPerRepository(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	ctx := context.Background()
	if err := fixture.repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:           "git-status-frontend-snapshot",
		SessionID:    "git-status-canonical",
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Files:        map[string]interface{}{"frontend.go": map[string]interface{}{"status": "modified"}},
		Metadata: map[string]interface{}{
			"repository_name": "frontend",
			"timestamp":       "2026-08-30T12:04:00Z",
			"modified":        []string{"frontend.go"},
		},
		CreatedAt: time.Date(2026, 8, 30, 12, 4, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create frontend snapshot: %v", err)
	}

	msgs := appendLiveGitStatusMessage(ctx, fixture.repo, nil, fixture.requested.ID, fixture.requested, nil, newTestLogger())
	if len(msgs) != 2 {
		t.Fatalf("expected one message per repository, got %d", len(msgs))
	}
	seen := make(map[string]map[string]interface{}, len(msgs))
	for _, msg := range msgs {
		payload := decodePayload(t, msg.Payload)
		status, ok := payload["status"].(map[string]interface{})
		if !ok {
			t.Fatalf("status payload = %#v, want object", payload["status"])
		}
		repositoryName, _ := status["repository_name"].(string)
		files, ok := status["files"].(map[string]interface{})
		if !ok {
			t.Fatalf("status files = %#v, want object", status["files"])
		}
		seen[repositoryName] = files
	}
	if _, ok := seen[""]["canonical.go"]; !ok {
		t.Fatalf("root repository snapshot missing: %#v", seen)
	}
	if _, ok := seen["frontend"]["frontend.go"]; !ok {
		t.Fatalf("frontend repository snapshot missing: %#v", seen)
	}
	if _, ok := seen[""]["mismatched.go"]; ok {
		t.Fatalf("mismatched repository snapshot selected: %#v", seen)
	}
}

func TestTryGetLiveGitStatusUsesSingleTimeoutAcrossSources(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	log := newTestLogger()
	firstClient, closeFirst := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer closeFirst()
	secondCalled := make(chan struct{}, 1)
	secondClient, closeSecond := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case secondCalled <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{
			Success: true,
			Repos: []client.PerRepoGitStatus{{Status: client.GitStatusResult{
				Success:   true,
				Timestamp: "2026-08-30T12:04:00Z",
				Files:     map[string]interface{}{"should-not-be-used.go": map[string]interface{}{"status": "modified"}},
			}}},
		})
	}))
	defer closeSecond()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	for _, item := range []struct {
		id     string
		client *client.Client
	}{
		{id: fixture.requested.ID, client: firstClient},
		{id: "git-status-canonical", client: secondClient},
	} {
		if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
			ID:                item.id + "-execution",
			TaskID:            fixture.requested.TaskID,
			SessionID:         item.id,
			TaskEnvironmentID: fixture.env.ID,
			WorkspacePath:     fixture.env.WorkspacePath,
		}); err != nil {
			t.Fatalf("add execution %s: %v", item.id, err)
		}
		execution, ok := mgr.GetExecutionBySessionID(item.id)
		if !ok {
			t.Fatalf("execution %s was not registered", item.id)
		}
		execution.SetAgentCtlClientForTesting(item.client)
	}

	sources := &gitStatusSources{
		environmentID: fixture.env.ID,
		workspacePath: fixture.env.WorkspacePath,
		sessionIDs:    []string{fixture.requested.ID, "git-status-canonical"},
	}
	msgs := tryGetLiveGitStatus(context.Background(), mgr, fixture.requested.ID, sources, log)
	if len(msgs) != 0 {
		t.Fatalf("expected the shared timeout to stop live probing after the first source, got %d messages", len(msgs))
	}
	select {
	case <-secondCalled:
		t.Fatal("second source was probed after the shared live-status deadline expired")
	default:
	}
}

func TestSessionGitRefreshPreservesMixedRepositoryOutcomes(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	log := newTestLogger()
	agentClient, closeServer := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{Success: true, Repos: []client.PerRepoGitStatus{
			{RepositoryName: "backend", Status: client.GitStatusResult{
				Success: true, StatusState: "ready", FilesComplete: true, DetailState: "pending",
				TrackerID:      "agentctl/tracker-3",
				RepositoryName: "backend", Modified: []string{"main.go"}, Files: map[string]interface{}{"main.go": map[string]interface{}{"status": "modified"}},
			}},
			{RepositoryName: "web", Status: client.GitStatusResult{
				Success: false, Error: "/private/path leaked", ErrorCode: "repository_unavailable",
			}},
		}})
	}))
	defer closeServer()
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID: "refresh-source", TaskID: fixture.requested.TaskID, SessionID: "git-status-canonical",
		TaskEnvironmentID: fixture.env.ID, WorkspacePath: fixture.env.WorkspacePath,
	}); err != nil {
		t.Fatalf("add source execution: %v", err)
	}
	execution, ok := mgr.GetExecutionBySessionID("git-status-canonical")
	if !ok {
		t.Fatal("source execution was not registered")
	}
	execution.SetAgentCtlClientForTesting(agentClient)

	result, err := buildSessionGitRefreshProvider(fixture.repo, mgr, log)(context.Background(), fixture.requested.ID, "fresh")
	if err != nil {
		t.Fatalf("refresh provider: %v", err)
	}
	if !result.Success || result.StatusState != "ready" || result.TaskEnvironmentID != fixture.env.ID || len(result.Snapshots) != 2 {
		t.Fatalf("refresh result = %+v, want two scoped repository states with one usable snapshot", result)
	}
	healthy := decodePayload(t, result.Snapshots[0].Payload)["status"].(map[string]interface{})
	if healthy["files_complete"] != true || healthy["detail_state"] != "pending" || healthy["tracker_epoch"] != float64(0) || healthy["tracker_id"] != "agentctl/tracker-3" {
		t.Fatalf("healthy status projection = %#v", healthy)
	}
	failed := decodePayload(t, result.Snapshots[1].Payload)["status"].(map[string]interface{})
	if failed["status_state"] != "unavailable" || failed["files_complete"] != false || failed["error_code"] != "repository_unavailable" {
		t.Fatalf("failed status projection = %#v", failed)
	}
	if _, leaked := failed["error"]; leaked {
		t.Fatal("per-repository Git error text was exposed in refresh data")
	}
}

func TestSessionGitRefreshAcceptsRegisteredRepoWorkspaceAfterRootPromotion(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	ctx := context.Background()
	repoPath := filepath.Join(fixture.env.WorkspacePath, "main-repository")
	if err := fixture.repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID:                "git-status-main-repo",
		TaskEnvironmentID: fixture.env.ID,
		RepositoryID:      "git-status-repository",
		WorktreePath:      repoPath,
	}); err != nil {
		t.Fatalf("create registered environment repository: %v", err)
	}
	if _, err := fixture.repo.DB().ExecContext(ctx, `
		UPDATE task_sessions SET workspace_path = ? WHERE id = ?
	`, repoPath, "git-status-canonical"); err != nil {
		t.Fatalf("preserve pre-promotion session workspace path: %v", err)
	}
	log := newTestLogger()
	agentClient, closeServer := newGitStatusClient(t, log, client.GitStatusResult{
		Success: true, StatusState: "ready", FilesComplete: true, DetailState: "ready",
		Modified: []string{"main.go"}, Files: map[string]interface{}{"main.go": map[string]interface{}{"status": "modified"}},
	})
	defer closeServer()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID: "git-status-repo-worktree-execution", TaskID: fixture.requested.TaskID,
		SessionID: "git-status-canonical", TaskEnvironmentID: fixture.env.ID, WorkspacePath: repoPath,
	}); err != nil {
		t.Fatalf("add execution at registered repository worktree: %v", err)
	}
	execution, ok := mgr.GetExecutionBySessionID("git-status-canonical")
	if !ok {
		t.Fatal("registered repository execution was not found")
	}
	execution.SetAgentCtlClientForTesting(agentClient)

	result, err := buildSessionGitRefreshProvider(fixture.repo, mgr, log)(ctx, fixture.requested.ID, "fresh")
	if err != nil {
		t.Fatalf("refresh provider: %v", err)
	}
	if !result.Success || len(result.Snapshots) != 1 {
		t.Fatalf("refresh result = %+v, want status from the current task environment's registered worktree", result)
	}
	status := decodePayload(t, result.Snapshots[0].Payload)["status"].(map[string]interface{})
	files := status["files"].(map[string]interface{})
	if _, ok := files["main.go"]; !ok {
		t.Fatalf("registered worktree status is missing from refresh result: %#v", status)
	}
}

func TestSessionGitRefreshRejectsRepoWorkspaceRemovedDuringRequest(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	ctx := context.Background()
	repoPath := filepath.Join(fixture.env.WorkspacePath, "main-repository")
	if err := fixture.repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID:                "git-status-main-repo",
		TaskEnvironmentID: fixture.env.ID,
		RepositoryID:      "git-status-repository",
		WorktreePath:      repoPath,
	}); err != nil {
		t.Fatalf("create registered environment repository: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	log := newTestLogger()
	agentClient, closeServer := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{Success: true, Repos: []client.PerRepoGitStatus{{Status: client.GitStatusResult{
			Success: true, StatusState: "ready", FilesComplete: true, DetailState: "ready",
		}}}})
	}))
	defer closeServer()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID: "git-status-repo-worktree-execution", TaskID: fixture.requested.TaskID,
		SessionID: "git-status-canonical", TaskEnvironmentID: fixture.env.ID, WorkspacePath: repoPath,
	}); err != nil {
		t.Fatalf("add execution at registered repository worktree: %v", err)
	}
	execution, ok := mgr.GetExecutionBySessionID("git-status-canonical")
	if !ok {
		t.Fatal("registered repository execution was not found")
	}
	execution.SetAgentCtlClientForTesting(agentClient)

	type refreshResult struct {
		result gateways.SessionGitRefreshResult
		err    error
	}
	resultCh := make(chan refreshResult, 1)
	go func() {
		result, err := buildSessionGitRefreshProvider(fixture.repo, mgr, log)(ctx, fixture.requested.ID, "fresh")
		resultCh <- refreshResult{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not reach agentctl")
	}
	if _, err := fixture.repo.DB().ExecContext(ctx, `
		UPDATE task_environment_repos SET status = 'deleted', deleted_at = ? WHERE id = ?
	`, time.Now().UTC(), "git-status-main-repo"); err != nil {
		t.Fatalf("retire repository worktree during refresh: %v", err)
	}
	close(release)
	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			t.Fatalf("refresh provider: %v", outcome.err)
		}
		if outcome.result.Success || len(outcome.result.Snapshots) != 0 || outcome.result.ErrorCode != "live_source_unavailable" {
			t.Fatalf("retired repository source result = %+v, want no stale snapshots", outcome.result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not return after the worktree was retired")
	}
}

func TestSessionGitRefreshContinuesToHealthySiblingSource(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	ctx := context.Background()
	healthySessionID := "git-status-healthy-sibling"
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: healthySessionID, TaskID: fixture.requested.TaskID, TaskEnvironmentID: fixture.env.ID,
		WorkspacePath: fixture.env.WorkspacePath, State: models.TaskSessionStateWaitingForInput,
		StartedAt: time.Date(2026, 8, 30, 11, 5, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 30, 11, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create healthy sibling session: %v", err)
	}
	log := newTestLogger()
	failedClient, closeFailed := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{Repos: []client.PerRepoGitStatus{{RepositoryName: "", Status: client.GitStatusResult{
			Success: false, ErrorCode: "status_unavailable",
		}}}})
	}))
	defer closeFailed()
	healthyClient, closeHealthy := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{Success: true, Repos: []client.PerRepoGitStatus{{RepositoryName: "", Status: client.GitStatusResult{
			Success: true, StatusState: "ready", FilesComplete: true, DetailState: "pending",
			Modified: []string{"healthy.go"}, Files: map[string]interface{}{"healthy.go": map[string]interface{}{"status": "modified"}},
		}}}})
	}))
	defer closeHealthy()

	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	for _, item := range []struct {
		sessionID string
		client    *client.Client
	}{{"git-status-canonical", failedClient}, {healthySessionID, healthyClient}} {
		execution := &lifecycle.AgentExecution{
			ID: item.sessionID + "-execution", TaskID: fixture.requested.TaskID, SessionID: item.sessionID,
			TaskEnvironmentID: fixture.env.ID, WorkspacePath: fixture.env.WorkspacePath,
		}
		if err := mgr.ExecutionStoreForTesting().Add(execution); err != nil {
			t.Fatalf("add source execution %s: %v", item.sessionID, err)
		}
		execution.SetAgentCtlClientForTesting(item.client)
	}

	result, err := buildSessionGitRefreshProvider(fixture.repo, mgr, log)(ctx, fixture.requested.ID, "fresh")
	if err != nil {
		t.Fatalf("refresh provider: %v", err)
	}
	if !result.Success || len(result.Snapshots) != 2 {
		t.Fatalf("refresh result = %+v; want failed and healthy source snapshots", result)
	}
	first := decodePayload(t, result.Snapshots[0].Payload)["status"].(map[string]interface{})
	last := decodePayload(t, result.Snapshots[1].Payload)["status"].(map[string]interface{})
	if first["status_state"] != "unavailable" || last["files_complete"] != true {
		t.Fatalf("source snapshots = %#v / %#v; want unavailable then complete", first, last)
	}
}

func TestSessionGitRefreshRejectsReplacedSource(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	log := newTestLogger()
	started := make(chan struct{})
	release := make(chan struct{})
	agentClient, closeServer := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{Success: true, Repos: []client.PerRepoGitStatus{{Status: client.GitStatusResult{
			Success: true, StatusState: "ready", FilesComplete: true, DetailState: "ready",
		}}}})
	}))
	defer closeServer()
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	original := &lifecycle.AgentExecution{
		ID: "refresh-source-old", TaskID: fixture.requested.TaskID, SessionID: "git-status-canonical",
		TaskEnvironmentID: fixture.env.ID, WorkspacePath: fixture.env.WorkspacePath,
	}
	if err := mgr.ExecutionStoreForTesting().Add(original); err != nil {
		t.Fatalf("add source execution: %v", err)
	}
	original.SetAgentCtlClientForTesting(agentClient)
	type refreshResult struct {
		result gateways.SessionGitRefreshResult
		err    error
	}
	resultCh := make(chan refreshResult, 1)
	go func() {
		result, err := buildSessionGitRefreshProvider(fixture.repo, mgr, log)(context.Background(), fixture.requested.ID, "fresh")
		resultCh <- refreshResult{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("agentctl refresh request did not start")
	}
	mgr.ExecutionStoreForTesting().Remove(original.ID)
	replacement := &lifecycle.AgentExecution{
		ID: "refresh-source-new", TaskID: fixture.requested.TaskID, SessionID: "git-status-canonical",
		TaskEnvironmentID: fixture.env.ID, WorkspacePath: fixture.env.WorkspacePath,
	}
	if err := mgr.ExecutionStoreForTesting().Add(replacement); err != nil {
		t.Fatalf("replace source execution: %v", err)
	}
	close(release)
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("refresh provider: %v", result.err)
		}
		if result.result.Success || len(result.result.Snapshots) != 0 || result.result.ErrorCode != "live_source_unavailable" {
			t.Fatalf("replaced source result = %+v, want unavailable without stale snapshots", result.result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("refresh provider did not return after source replacement")
	}
}

func TestAppendLiveGitStatusMessageDoesNotFallbackForUnverifiedEnvironment(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	fixture.env.WorkspacePath = "/tasks/git-status/unverified"
	if err := fixture.repo.UpdateTaskEnvironment(context.Background(), fixture.env); err != nil {
		t.Fatalf("UpdateTaskEnvironment: %v", err)
	}

	msgs := appendLiveGitStatusMessage(
		context.Background(), fixture.repo, nil, fixture.requested.ID, fixture.requested, nil, newTestLogger(),
	)
	if len(msgs) != 0 {
		t.Fatalf("expected no status for an unverified environment, got %d messages", len(msgs))
	}
}

func newGitStatusClient(t *testing.T, log *logger.Logger, status client.GitStatusResult) (*client.Client, func()) {
	t.Helper()
	return newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/git/status/multi" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{
			Success: true,
			Repos:   []client.PerRepoGitStatus{{Status: status}},
		})
	}))
}

func newGitStatusClientWithHandler(t *testing.T, log *logger.Logger, handler http.Handler) (*client.Client, func()) {
	t.Helper()
	server := httptest.NewServer(handler)

	parsed, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("parse test agentctl URL: %v", err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		server.Close()
		t.Fatalf("split test agentctl address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		server.Close()
		t.Fatalf("parse test agentctl port: %v", err)
	}
	return client.NewClient(host, port, log), server.Close
}
