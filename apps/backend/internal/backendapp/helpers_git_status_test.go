package backendapp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
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
