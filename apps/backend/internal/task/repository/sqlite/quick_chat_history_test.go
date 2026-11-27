package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func seedQuickChatHistory(t *testing.T, repo *Repository, sessionID string, ephemeral bool, runtime agentruntime.Runtime) {
	t.Helper()
	ctx := context.Background()
	workspaceID := sessionID + "-workspace"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "History"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: sessionID + "-task", WorkspaceID: workspaceID, Title: "History", IsEphemeral: ephemeral}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: sessionID + "-task", AgentProfileSnapshot: map[string]interface{}{"agent_name": quickChatHistoryClaudeAgent},
		ExecutionProfileID: "history-profile",
		Metadata:           map[string]interface{}{"unrelated": "keep"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: sessionID, TaskID: sessionID + "-task", AgentExecutionID: sessionID + "-execution",
		Runtime: runtime, ResumeToken: "token-a", WorktreePath: "/w/chat",
		ExecutionProfileID: "history-profile",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQuickChatHistoryKeepsProviderAndCwdAcrossProfileRotation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedQuickChatHistory(t, repo, "profile-history", true, agentruntime.RuntimeStandalone)
	session, err := repo.GetTaskSession(ctx, "profile-history")
	if err != nil {
		t.Fatal(err)
	}
	session.ExecutionProfileID = "next-profile"
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "codex-app-server"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: session.ID, TaskID: session.TaskID, AgentExecutionID: "next-execution",
		ExecutionProfileID: "next-profile", Runtime: agentruntime.RuntimeStandalone,
		ResumeToken: "token-b", WorktreePath: "/w/next",
	}); err != nil {
		t.Fatal(err)
	}
	session, err = repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(session.Metadata["quick_chat_agent_history"])
	if err != nil {
		t.Fatal(err)
	}
	var records []models.QuickChatAgentHistory
	if err := json.Unmarshal(raw, &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].AgentName != quickChatHistoryClaudeAgent || records[0].WorkDir != "/w/chat" ||
		records[1].AgentName != "codex-app-server" || records[1].WorkDir != "/w/next" {
		t.Fatalf("provider rotation relabelled history: %s", raw)
	}
}

func quickChatHistoryTokens(t *testing.T, repo *Repository, sessionID string) map[string]int {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Metadata["unrelated"] != "keep" {
		t.Fatal("history mutation discarded unrelated session metadata")
	}
	raw, err := json.Marshal(session.Metadata["quick_chat_agent_history"])
	if err != nil {
		t.Fatal(err)
	}
	var records []struct {
		AgentName   string `json:"agent_name"`
		ResumeToken string `json:"resume_token"`
		WorkDir     string `json:"work_dir"`
	}
	if err := json.Unmarshal(raw, &records); err != nil {
		t.Fatal(err)
	}
	tokens := make(map[string]int)
	for _, record := range records {
		if record.AgentName != quickChatHistoryClaudeAgent || record.WorkDir != "/w/chat" {
			t.Fatalf("history identity lost its provider or cwd: %+v", record)
		}
		tokens[record.ResumeToken]++
	}
	return tokens
}

func TestQuickChatHistoryDoesNotRenewIdleActivity(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedQuickChatHistory(t, repo, "idle-history", true, agentruntime.RuntimeStandalone)
	idleSince := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET updated_at = ? WHERE id = ?`, idleSince, "idle-history"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateResumeToken(ctx, "idle-history", "idle-history-execution", "token-b", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: "idle-history", TaskID: "idle-history-task", AgentExecutionID: "idle-history-execution",
		ExecutionProfileID: "history-profile", Runtime: agentruntime.RuntimeStandalone,
		ResumeToken: "token-c", WorktreePath: "/w/chat",
	}); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "idle-history")
	if err != nil {
		t.Fatal(err)
	}
	if !session.UpdatedAt.Equal(idleSince) {
		t.Fatalf("history bookkeeping renewed idle activity: %s, want %s", session.UpdatedAt, idleSince)
	}
	if tokens := quickChatHistoryTokens(t, repo, session.ID); len(tokens) != 3 {
		t.Fatalf("history retention lost accepted identities: %v", tokens)
	}
}

func TestQuickChatHistorySurvivesTokenCASClearAndExecutorDeletion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedQuickChatHistory(t, repo, "quick-history", true, agentruntime.RuntimeStandalone)
	if err := repo.UpdateResumeToken(ctx, "quick-history", "quick-history-execution", "token-b", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateResumeToken(ctx, "quick-history", "stale-execution", "stale-token", ""); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("stale token update = %v", err)
	}
	if err := repo.UpdateResumeToken(ctx, "quick-history", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteExecutorRunningBySessionID(ctx, "quick-history"); err != nil {
		t.Fatal(err)
	}
	tokens := quickChatHistoryTokens(t, repo, "quick-history")
	if len(tokens) != 2 || tokens["token-a"] != 1 || tokens["token-b"] != 1 {
		t.Fatalf("retained history = %v, want each accepted identity once", tokens)
	}
}

func TestQuickChatHistoryExcludesNormalTasksAndRemoteRuntimes(t *testing.T) {
	repo := newRepoForSessionTests(t)
	for _, fixture := range []struct {
		id        string
		ephemeral bool
		runtime   agentruntime.Runtime
	}{{"normal-history", false, agentruntime.RuntimeStandalone}, {"remote-history", true, agentruntime.RuntimeSSH}} {
		seedQuickChatHistory(t, repo, fixture.id, fixture.ephemeral, fixture.runtime)
		if err := repo.UpdateResumeToken(context.Background(), fixture.id, "", "next-token", ""); err != nil {
			t.Fatal(err)
		}
		if tokens := quickChatHistoryTokens(t, repo, fixture.id); len(tokens) != 0 {
			t.Fatalf("ineligible history recorded for %s: %v", fixture.id, tokens)
		}
	}
}

func TestQuickChatHistoryExcludesManagedAndAutomatedTasks(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	for _, id := range []string{"managed-history", "automation-history", "coordinator-history", "workflow-history"} {
		seedQuickChatHistory(t, repo, id, true, agentruntime.RuntimeSSH)
		task, err := repo.GetTask(ctx, id+"-task")
		if err != nil {
			t.Fatal(err)
		}
		switch id {
		case "managed-history":
			task.Metadata = map[string]interface{}{"kandev.plugin_id": "plugin"}
		case "automation-history":
			task.Origin = models.TaskOriginAutomationRun
		case "coordinator-history":
			task.Origin = models.TaskOriginCoordinator
		case "workflow-history":
			if err := repo.CreateWorkflow(ctx, &models.Workflow{
				ID: "history-workflow", WorkspaceID: id + "-workspace", Name: "Workflow",
			}); err != nil {
				t.Fatal(err)
			}
			task.WorkflowID = "history-workflow"
		}
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			SessionID: id, TaskID: id + "-task", Runtime: agentruntime.RuntimeStandalone,
			ResumeToken: "local-token", WorktreePath: "/w/chat",
		}); err != nil {
			t.Fatal(err)
		}
		if tokens := quickChatHistoryTokens(t, repo, id); len(tokens) != 0 {
			t.Fatalf("ineligible history recorded for %s: %v", id, tokens)
		}
	}
}

func TestQuickChatHistoryDoesNotAttributeForeignExecutorRow(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedQuickChatHistory(t, repo, "foreign-history", true, agentruntime.RuntimeSSH)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: "foreign-history", TaskID: "different-task", Runtime: agentruntime.RuntimeStandalone,
		ResumeToken: "foreign-token", WorktreePath: "/w/foreign",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateResumeToken(ctx, "foreign-history", "", "another-foreign-token", ""); err != nil {
		t.Fatal(err)
	}
	if tokens := quickChatHistoryTokens(t, repo, "foreign-history"); len(tokens) != 0 {
		t.Fatalf("foreign executor history attributed to session: %v", tokens)
	}
}

func TestPostgresQuickChatHistoryCAS(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	seedQuickChatHistory(t, repo, "quick-history-pg", true, agentruntime.RuntimeStandalone)
	idleSince := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(context.Background(), db.Rebind(`UPDATE task_sessions SET updated_at = ? WHERE id = ?`), idleSince, "quick-history-pg"); err != nil {
		t.Fatal(err)
	}
	secondDB := openSecondPostgresConnection(t, dsn, db)
	secondRepo, err := NewWithDB(secondDB, secondDB, nil)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for i, writer := range []*Repository{repo, secondRepo} {
		group.Add(1)
		go func(writer *Repository, token string) {
			defer group.Done()
			<-start
			errs <- writer.UpdateResumeToken(context.Background(), "quick-history-pg", "quick-history-pg-execution", token, "")
		}(writer, []string{"token-b", "token-c"}[i])
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	tokens := quickChatHistoryTokens(t, repo, "quick-history-pg")
	if len(tokens) != 3 || tokens["token-a"] != 1 || tokens["token-b"] != 1 || tokens["token-c"] != 1 {
		t.Fatalf("concurrent accepted identities = %v", tokens)
	}
	session, err := repo.GetTaskSession(context.Background(), "quick-history-pg")
	if err != nil {
		t.Fatal(err)
	}
	if !session.UpdatedAt.Equal(idleSince) {
		t.Fatalf("concurrent history writes renewed idle activity: %s, want %s", session.UpdatedAt, idleSince)
	}
}
