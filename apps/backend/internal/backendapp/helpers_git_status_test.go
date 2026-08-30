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
		Files:        map[string]interface{}{},
		Metadata: map[string]interface{}{
			"timestamp": "2026-08-30T12:01:00Z",
			"modified":  []string{},
		},
		CreatedAt: now.Add(2 * time.Minute),
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
		ID:                "git-status-canonical-execution",
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

func TestAppendLiveGitStatusMessageRejectsMismatchedLiveExecution(t *testing.T) {
	fixture := newGitStatusEnvironmentFixture(t)
	log := newTestLogger()
	client, closeServer := newGitStatusClient(t, log, client.GitStatusResult{
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
	execution.SetAgentCtlClientForTesting(client)

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
