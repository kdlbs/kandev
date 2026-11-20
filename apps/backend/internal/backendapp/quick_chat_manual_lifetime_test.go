package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUICK-CHAT-EXPIRATION-003.1, .2, .7
func TestQuickChatManualLifetimeAcrossStartupAndRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	databasePath := filepath.Join(root, "kandev.db")
	quickChatDir := filepath.Join(root, "quick-chat")
	historyHome := t.TempDir()
	t.Setenv("HOME", historyHome)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")

	const taskID = "quick-chat-old"
	const sessionID = "quick-chat-old-session"
	const messageID = "quick-chat-old-message"
	const resumeToken = "ffffffff-0000-4000-8000-000000000001"
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	seedQuickChatManualLifetime(t, databasePath, quickChatDir, historyHome, taskID, sessionID, messageID, resumeToken, old)

	for startup := 0; startup < 2; startup++ {
		dbConn, repo, svc, closeDatabase := openQuickChatManualLifetimeDatabase(t, databasePath, quickChatDir)
		var closeOnce sync.Once
		close := func() { closeOnce.Do(closeDatabase) }
		t.Cleanup(close)
		serviceCtx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)
		startTaskBackgroundWorkers(serviceCtx, svc, 2*time.Hour)
		require.Never(t, func() bool {
			_, err := repo.GetTask(ctx, taskID)
			return err != nil
		}, 2*time.Second, 10*time.Millisecond, "startup must not delete an aged Quick Chat")
		assertQuickChatManualLifetime(t, ctx, dbConn, repo, taskID, sessionID, messageID, quickChatDir, historyHome, resumeToken)
		cancel()
		close()
	}
}

func seedQuickChatManualLifetime(
	t *testing.T,
	databasePath, quickChatDir, historyHome, taskID, sessionID, messageID, resumeToken string,
	old time.Time,
) {
	t.Helper()
	_, repo, _, closeDatabase := openQuickChatManualLifetimeDatabase(t, databasePath, quickChatDir)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-quick-chat", Name: "Quick Chat"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-quick-chat", Title: taskID, State: v1.TaskStateTODO,
		Priority: "medium", IsEphemeral: true, CreatedAt: old.Add(-time.Hour), UpdatedAt: old,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?`,
		old.Add(-time.Hour), old, taskID); err != nil {
		t.Fatalf("backdate task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCompleted,
		StartedAt: old.Add(-time.Hour), UpdatedAt: old, IsPrimary: true,
		AgentProfileSnapshot: map[string]interface{}{"agent_name": "claude-acp"},
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, ExecutorID: "exec-local",
		Runtime: "standalone", Status: models.ExecutorRunningStatusStopped,
		ResumeToken: resumeToken, WorktreePath: filepath.Join(quickChatDir, sessionID),
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "quick-chat-old-turn", TaskID: taskID, TaskSessionID: sessionID,
		StartedAt: old, CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: messageID, TaskID: taskID, TaskSessionID: sessionID, TurnID: "quick-chat-old-turn",
		AuthorType: models.MessageAuthorAgent, Content: "conversation history", CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET updated_at = ? WHERE id = ?`, old, sessionID); err != nil {
		t.Fatalf("backdate session: %v", err)
	}
	worktree := filepath.Join(quickChatDir, sessionID)
	if err := os.MkdirAll(worktree, 0o700); err != nil {
		t.Fatalf("MkdirAll Quick Chat workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "owned-resource"), []byte("preserve"), 0o600); err != nil {
		t.Fatalf("WriteFile owned resource: %v", err)
	}
	transcript := quickChatClaudeTranscriptPath(historyHome, worktree, resumeToken)
	if err := os.MkdirAll(filepath.Dir(transcript), 0o700); err != nil {
		t.Fatalf("MkdirAll agent history: %v", err)
	}
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile agent history: %v", err)
	}
	closeDatabase()
}

func openQuickChatManualLifetimeDatabase(
	t *testing.T,
	databasePath, quickChatDir string,
) (*sqlx.DB, *sqliterepo.Repository, *taskservice.Service, func()) {
	t.Helper()
	dbConn, err := db.OpenSQLite(databasePath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, repoCleanup, err := taskrepo.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		_ = sqlxDB.Close()
		t.Fatalf("task repository: %v", err)
	}
	log := testLogger(t)
	t.Cleanup(func() { _ = log.Close() })
	svc := taskservice.NewService(taskservice.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, WorkspaceFolders: repo,
		Workflows: repo, Messages: repo, Turns: repo, Sessions: repo,
		GitSnapshots: repo, RepoEntities: repo, RepositorySets: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo, ResourceCleanups: repo, StatusSummaries: repo,
		RecoveryOperations: repo,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})
	svc.SetQuickChatDir(quickChatDir)
	var closeOnce sync.Once
	closeDatabase := func() {
		closeOnce.Do(func() {
			if err := sqlxDB.Close(); err != nil {
				t.Errorf("close database: %v", err)
			}
			if err := repoCleanup(); err != nil {
				t.Errorf("close task repository: %v", err)
			}
		})
	}
	t.Cleanup(closeDatabase)
	return sqlxDB, repo, svc, closeDatabase
}

func assertQuickChatManualLifetime(
	t *testing.T,
	ctx context.Context,
	dbConn *sqlx.DB,
	repo *sqliterepo.Repository,
	taskID, sessionID, messageID, quickChatDir, historyHome, resumeToken string,
) {
	t.Helper()
	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err, "task should survive startup")
	require.Equal(t, taskID, task.ID)
	workspace, err := repo.GetWorkspace(ctx, task.WorkspaceID)
	require.NoError(t, err, "workspace should survive startup")
	require.Equal(t, "ws-quick-chat", workspace.ID)
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, sessionID, sessions[0].ID)
	var messages []struct {
		ID string `db:"id"`
	}
	require.NoError(t, dbConn.SelectContext(ctx, &messages,
		"SELECT id FROM task_session_messages WHERE task_id = ? ORDER BY id", taskID))
	require.Equal(t, []struct {
		ID string `db:"id"`
	}{{ID: messageID}}, messages)
	require.FileExists(t, filepath.Join(quickChatDir, sessionID, "owned-resource"))
	require.FileExists(t, quickChatClaudeTranscriptPath(historyHome, filepath.Join(quickChatDir, sessionID), resumeToken))
}

func quickChatClaudeTranscriptPath(home, worktree, resumeToken string) string {
	var project strings.Builder
	for _, char := range worktree {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			project.WriteRune(char)
		} else {
			project.WriteByte('-')
		}
	}
	return filepath.Join(home, ".claude", "projects", project.String(), resumeToken+".jsonl")
}
