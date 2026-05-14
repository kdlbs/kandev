package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func TestGetWorkspaceInfoForSession_BasicFields(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		AgentProfileID:    "profile-1",
		State:             models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		Metadata: map[string]interface{}{
			"acp_session_id": "acp-123",
		},
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Add a worktree to the session
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID:             "wt1",
		SessionID:      "session-1",
		WorktreeID:     "wid1",
		RepositoryID:   "repo1",
		WorktreePath:   "/tmp/worktrees/session-1",
		WorktreeBranch: "feature/test",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %q", info.TaskID)
	}
	if info.SessionID != "session-1" {
		t.Errorf("expected SessionID 'session-1', got %q", info.SessionID)
	}
	if info.TaskEnvironmentID != "env-123" {
		t.Errorf("expected TaskEnvironmentID 'env-123', got %q", info.TaskEnvironmentID)
	}
	if info.WorkspacePath != "/tmp/worktrees/session-1" {
		t.Errorf("expected WorkspacePath '/tmp/worktrees/session-1', got %q", info.WorkspacePath)
	}
	if info.AgentProfileID != "profile-1" {
		t.Errorf("expected AgentProfileID 'profile-1', got %q", info.AgentProfileID)
	}
	if info.AgentID != "auggie" {
		t.Errorf("expected AgentID 'auggie', got %q", info.AgentID)
	}
	if info.ACPSessionID != "acp-123" {
		t.Errorf("expected ACPSessionID 'acp-123', got %q", info.ACPSessionID)
	}
}

func TestGetWorkspaceInfoForSession_InfersTaskID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Pass empty taskID - should be inferred from the session
	info, err := svc.GetWorkspaceInfoForSession(ctx, "", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}
	if info.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123' inferred from session, got %q", info.TaskID)
	}
}

func TestGetWorkspaceInfoForSession_ExecutorInfo(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	// Create executor
	exec := &models.Executor{
		ID:        "exec-1",
		Name:      "My Sprites Executor",
		Type:      models.ExecutorTypeSprites,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateExecutor(ctx, exec); err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	// Create session with executor reference
	session := &models.TaskSession{
		ID:         "session-1",
		TaskID:     "task-123",
		ExecutorID: "exec-1",
		State:      models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Create executor running record
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er-1",
		SessionID:        "session-1",
		TaskID:           "task-123",
		ExecutorID:       "exec-1",
		Runtime:          "sprites",
		AgentExecutionID: "agent-exec-abc123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.ExecutorType != "sprites" {
		t.Errorf("expected ExecutorType 'sprites', got %q", info.ExecutorType)
	}
	if info.RuntimeName != "sprites" {
		t.Errorf("expected RuntimeName 'sprites', got %q", info.RuntimeName)
	}
	if info.AgentExecutionID != "agent-exec-abc123" {
		t.Errorf("expected AgentExecutionID 'agent-exec-abc123', got %q", info.AgentExecutionID)
	}
}

func TestGetWorkspaceInfoForSession_IncludesEnvironmentReconnectMetadata(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID:        "exec-1",
		Name:      "Docker",
		Type:      models.ExecutorTypeLocalDocker,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            "env-123",
		TaskID:        "task-123",
		ExecutorType:  string(models.ExecutorTypeLocalDocker),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: "/host/repo",
		ContainerID:   "container-from-env",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("failed to create task environment: %v", err)
	}

	session := &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		ExecutorID:        "exec-1",
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "codex",
		},
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er-1",
		SessionID:        "session-1",
		TaskID:           "task-123",
		ExecutorID:       "exec-1",
		Runtime:          "docker",
		AgentExecutionID: "running-exec",
		ContainerID:      "container-from-running",
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret:      "secret-token",
			lifecycle.MetadataKeyBootstrapNonceSecret: "secret-nonce",
			lifecycle.MetadataKeyImageTagOverride:     "kandev:test",
			"task_description":                        "drop me",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.WorkspacePath != "/host/repo" {
		t.Fatalf("WorkspacePath = %q, want /host/repo", info.WorkspacePath)
	}
	if info.AgentExecutionID != "running-exec" {
		t.Fatalf("AgentExecutionID = %q, want running-exec", info.AgentExecutionID)
	}
	if info.Metadata[lifecycle.MetadataKeyContainerID] != "container-from-running" {
		t.Fatalf("container metadata = %v, want container-from-running", info.Metadata[lifecycle.MetadataKeyContainerID])
	}
	if info.Metadata[lifecycle.MetadataKeyAuthTokenSecret] != "secret-token" {
		t.Fatalf("auth secret metadata missing: %v", info.Metadata)
	}
	if info.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] != "secret-nonce" {
		t.Fatalf("nonce secret metadata missing: %v", info.Metadata)
	}
	if info.Metadata[lifecycle.MetadataKeyImageTagOverride] != "kandev:test" {
		t.Fatalf("image override metadata missing: %v", info.Metadata)
	}
	if _, ok := info.Metadata["task_description"]; ok {
		t.Fatalf("launch-only metadata should not be retained: %v", info.Metadata)
	}
}

func TestGetWorkspaceInfoForSession_NoExecutorRunning(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// No executor running record - should still succeed with empty executor fields
	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}
	if info.RuntimeName != "" {
		t.Errorf("expected empty RuntimeName, got %q", info.RuntimeName)
	}
	if info.AgentExecutionID != "" {
		t.Errorf("expected empty AgentExecutionID, got %q", info.AgentExecutionID)
	}
	if info.ExecutorType != "" {
		t.Errorf("expected empty ExecutorType, got %q", info.ExecutorType)
	}
}

// TestGetWorkspaceInfoForSession_AlignsTaskEnvironmentIDOnFallback locks in
// the fix in applyTaskEnvironmentToWorkspaceInfo: when the session's stored
// TaskEnvironmentID points to a missing/stale row, GetWorkspaceInfoForSession
// falls back to GetTaskEnvironmentByTaskID. The previous implementation kept
// info.TaskEnvironmentID pointing at the stale ID while picking up
// metadata/workspace path from the fallback env — a downstream ID/metadata
// mismatch that broke reconciler keying. The fix always aligns the ID with
// the env we resolved against.
func TestGetWorkspaceInfoForSession_AlignsTaskEnvironmentIDOnFallback(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	// Real env exists under a different ID than what the session points at.
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            "real-env",
		TaskID:        "task-123",
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: "/host/real-env",
		ContainerID:   "container-real",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("failed to create task environment: %v", err)
	}

	// Session stores a stale TaskEnvironmentID — the env it points at has
	// been deleted or rolled over, but the session row still references it.
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "stale-env",
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.TaskEnvironmentID != "real-env" {
		t.Fatalf("TaskEnvironmentID = %q, want real-env (alignment fix should overwrite the session's stale ID)", info.TaskEnvironmentID)
	}
	if info.WorkspacePath != "/host/real-env" {
		t.Fatalf("WorkspacePath = %q, want /host/real-env (metadata must come from the resolved env, same as the ID)", info.WorkspacePath)
	}
	if got := info.Metadata["container_id"]; got != "container-real" {
		t.Fatalf("metadata container_id = %v, want container-real", got)
	}
}

func TestGetWorkspaceInfoForSession_SessionNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	_, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestGetWorkspaceInfoForEnvironment(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:        "env-123",
		TaskID:    "task-123",
		Status:    models.TaskEnvironmentStatusReady,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create task environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		State:             models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-2",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		State:             models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("failed to create newer session: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForEnvironment(ctx, "env-123")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForEnvironment returned error: %v", err)
	}
	if info.SessionID != "session-2" {
		t.Errorf("SessionID = %q, want session-2", info.SessionID)
	}
	if info.TaskEnvironmentID != "env-123" {
		t.Errorf("TaskEnvironmentID = %q, want env-123", info.TaskEnvironmentID)
	}
}

// Multi-repo: the workspace path agentctl boots with must be the task root
// (parent of every per-repo subdir) so its scanRepositorySubdirs detects all
// repos and starts a per-repo tracker for each. Returning a single repo's
// path would collapse fan-out into the legacy single-tracker mode and
// suppress the per-repo events the Changes panel needs to render headers.
func TestGetWorkspaceInfoForSession_MultiRepoReturnsTaskRoot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-multi",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	for i, path := range []string{
		"/tmp/tasks/do-nothing_mvo/kandev",
		"/tmp/tasks/do-nothing_mvo/thm",
	} {
		if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
			ID:           fmt.Sprintf("wt%d", i),
			SessionID:    session.ID,
			WorktreeID:   fmt.Sprintf("wid%d", i),
			RepositoryID: fmt.Sprintf("repo%d", i),
			Position:     i,
			WorktreePath: path,
			CreatedAt:    now,
		}); err != nil {
			t.Fatalf("create worktree %d: %v", i, err)
		}
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", session.ID)
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if info.WorkspacePath != "/tmp/tasks/do-nothing_mvo" {
		t.Errorf("expected WorkspacePath '/tmp/tasks/do-nothing_mvo' (task root), got %q",
			info.WorkspacePath)
	}
}

// AbandonOpenTurns: turns left open by a previous crash must close with
// completed_at = started_at so analytics' active_duration_ms doesn't get
// poisoned with the dead window and the UI's running timer doesn't count
// from a stale start.
func TestAbandonOpenTurns_ZeroesDuration(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	stale := &models.Turn{
		ID:            "turn-stale",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		StartedAt:     time.Now().Add(-90 * time.Hour).UTC(),
	}
	if err := repo.CreateTurn(ctx, stale); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	got, err := repo.GetTurn(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
	if !got.CompletedAt.Equal(got.StartedAt) {
		t.Fatalf("expected completed_at == started_at (zero duration), got started=%v completed=%v",
			got.StartedAt, *got.CompletedAt)
	}
}

// AbandonOpenTurns sweeps the same pending tool calls that CompleteTurn does:
// otherwise the UI would show "running" tool calls forever on an abandoned turn.
func TestAbandonOpenTurns_CompletesPendingToolCalls(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	turn := &models.Turn{
		ID:            "turn-with-tool",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		StartedAt:     time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	toolMsg := &models.Message{
		ID:            "msg-tool-1",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turn.ID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeToolCall,
		Content:       "running tool",
		Metadata: map[string]interface{}{
			"tool_call_id": "tc-1",
			"status":       "running",
		},
	}
	if err := repo.CreateMessage(ctx, toolMsg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	got, err := repo.GetMessageByToolCallID(ctx, sessionID, "tc-1")
	if err != nil {
		t.Fatalf("GetMessageByToolCallID: %v", err)
	}
	if got == nil {
		t.Fatal("expected tool call message to exist")
	}
	if status, _ := got.Metadata["status"].(string); status != "complete" {
		t.Fatalf("expected tool call status='complete', got %q", status)
	}
}

// AbandonOpenTurns publishes turn.completed for each closed turn so the WS
// gateway can broadcast the state change and the frontend clears its
// activeBySession entry.
func TestAbandonOpenTurns_PublishesTurnCompletedEvent(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	turn := &models.Turn{
		ID:            "turn-event",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		StartedAt:     time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	eventBus.ClearEvents()
	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	var found bool
	for _, ev := range eventBus.GetPublishedEvents() {
		if ev.Type == events.TurnCompleted {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected turn.completed event to be published")
	}
}

// AbandonOpenTurns is a no-op when no open turn exists (e.g. resume called on
// a session whose previous turn already completed cleanly).
func TestAbandonOpenTurns_NoOpenTurns(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	eventBus.ClearEvents()
	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	if got := len(eventBus.GetPublishedEvents()); got != 0 {
		t.Fatalf("expected no events when nothing to abandon, got %d", got)
	}
}

// AbandonOpenTurns iteration cap: with more than maxIterations open turns,
// the function still returns nil (caller swallows the error and the next
// completeTurnForSession sweep mops up the remainder).
func TestAbandonOpenTurns_IterationCap(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	const seeded = 20 // > maxIterations (16)
	for i := 0; i < seeded; i++ {
		turn := &models.Turn{
			ID:            fmt.Sprintf("turn-cap-%d", i),
			TaskSessionID: sessionID,
			TaskID:        "task-123",
			StartedAt:     time.Now().UTC(),
		}
		if err := repo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("seed turn %d: %v", i, err)
		}
	}

	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	// First sweep closes exactly maxIterations; remainder still open.
	turns, err := repo.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	open := 0
	for _, turn := range turns {
		if turn.CompletedAt == nil {
			open++
		}
	}
	if open != seeded-16 {
		t.Fatalf("expected %d open turns after first sweep (cap=16), got %d", seeded-16, open)
	}

	// Second sweep finishes the job.
	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("second AbandonOpenTurns: %v", err)
	}
	turns, _ = repo.ListTurnsBySession(ctx, sessionID)
	for _, turn := range turns {
		if turn.CompletedAt == nil {
			t.Fatalf("expected all turns closed after second sweep, %s still open", turn.ID)
		}
	}
}
