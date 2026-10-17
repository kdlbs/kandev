package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// claudeProjectName mirrors Claude Code's project directory naming for tests.
func claudeProjectName(workDir string) string {
	var b strings.Builder
	for _, r := range workDir {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

type agentHistoryFixture struct {
	home         string
	quickChatDir string
}

func newAgentHistoryFixture(t *testing.T, svc *Service) agentHistoryFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	quickChatDir := filepath.Join(t.TempDir(), "quick-chat")
	svc.SetQuickChatDir(quickChatDir)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	return agentHistoryFixture{home: home, quickChatDir: quickChatDir}
}

func (f agentHistoryFixture) writeClaudeTranscript(t *testing.T, workDir, token string) string {
	t.Helper()
	path := filepath.Join(f.home, ".claude", "projects", claudeProjectName(workDir), token+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f agentHistoryFixture) writeCodexRollout(t *testing.T, workDir, thread string) string {
	t.Helper()
	path := filepath.Join(f.home, ".codex", "sessions", "2026", "10", "14", "rollout-2026-10-14T10-00-00-"+thread+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"type":"session_meta","payload":{"id":"` + thread + `","cwd":"` + workDir + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type agentHistoryTaskSpec struct {
	taskID      string
	workflowID  string
	ephemeral   bool
	agentName   string
	resumeToken string
	worktree    string
	runtime     agentruntime.Runtime
	updatedAt   time.Time
}

func seedAgentHistoryTask(t *testing.T, repo *sqliterepo.Repository, spec agentHistoryTaskSpec) string {
	t.Helper()
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-history", Name: "History"})
	if spec.workflowID != "" {
		_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: spec.workflowID, WorkspaceID: "ws-history", Name: "Workflow"})
	}
	if spec.updatedAt.IsZero() {
		spec.updatedAt = time.Now().UTC()
	}
	if spec.runtime == "" {
		spec.runtime = agentruntime.RuntimeStandalone
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: spec.taskID, WorkspaceID: "ws-history", WorkflowID: spec.workflowID, Title: spec.taskID,
		State: v1.TaskStateTODO, Priority: "medium", IsEphemeral: spec.ephemeral,
		CreatedAt: spec.updatedAt.Add(-time.Hour), UpdatedAt: spec.updatedAt,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?`,
		spec.updatedAt.Add(-time.Hour), spec.updatedAt, spec.taskID); err != nil {
		t.Fatalf("backdate task: %v", err)
	}
	sessionID := spec.taskID + "-session"
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: spec.taskID, State: models.TaskSessionStateCompleted,
		StartedAt: spec.updatedAt.Add(-time.Hour), UpdatedAt: spec.updatedAt, IsPrimary: true,
		AgentProfileSnapshot: map[string]interface{}{"agent_name": spec.agentName},
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: spec.taskID, ExecutorID: "exec-local",
		Runtime: spec.runtime, Status: models.ExecutorRunningStatusStopped,
		ResumeToken: spec.resumeToken, WorktreePath: spec.worktree,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	return sessionID
}

func assertPathExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Lstat(path)
	if want && err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if !want && !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, got %v", path, err)
	}
}

func TestDeleteQuickChatRemovesOnlyItsClaudeHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	sessionID := seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-claude", ephemeral: true, agentName: "claude-acp", resumeToken: "aaaaaaaa-0000-4000-8000-000000000001",
	})
	scratch := filepath.Join(f.quickChatDir, sessionID)
	own := f.writeClaudeTranscript(t, scratch, "aaaaaaaa-0000-4000-8000-000000000001")
	sibling := f.writeClaudeTranscript(t, scratch, "aaaaaaaa-0000-4000-8000-000000000002")
	unrelated := f.writeClaudeTranscript(t, "/home/u/.kandev/tasks/clean-up-quick-chat_x/repo", "aaaaaaaa-0000-4000-8000-000000000001")

	if err := svc.DeleteTask(context.Background(), "quick-claude"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	assertPathExists(t, own, false)
	assertPathExists(t, sibling, true)
	assertPathExists(t, unrelated, true)
}

func TestDeleteRepositoryQuickChatRemovesCodexHistoryInWorktree(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	worktreeDir := "/home/u/.kandev/tasks/quick-repo_abc/repo"
	thread := "01a109d5-1957-7462-9235-e91c49e45533"
	seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-codex", ephemeral: true, agentName: "codex-acp", resumeToken: thread, worktree: worktreeDir,
	})
	own := f.writeCodexRollout(t, worktreeDir, thread)
	otherCwd := f.writeCodexRollout(t, "/home/u/project", "01a109d5-1957-7462-9235-e91c49e45534")

	if err := svc.DeleteTask(context.Background(), "quick-codex"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	assertPathExists(t, own, false)
	assertPathExists(t, otherCwd, true)
}

func TestDeleteNormalTaskKeepsAgentHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	worktreeDir := "/home/u/.kandev/tasks/normal_abc/repo"
	seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "normal-task", workflowID: "wf-history", agentName: "claude-acp",
		resumeToken: "bbbbbbbb-0000-4000-8000-000000000001", worktree: worktreeDir,
	})
	transcript := f.writeClaudeTranscript(t, worktreeDir, "bbbbbbbb-0000-4000-8000-000000000001")

	if err := svc.DeleteTask(context.Background(), "normal-task"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	assertPathExists(t, transcript, true)
}

func TestDeleteQuickChatOnRemoteRuntimeKeepsHostHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	worktreeDir := "/workspace"
	seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-docker", ephemeral: true, agentName: "claude-acp",
		resumeToken: "cccccccc-0000-4000-8000-000000000001", worktree: worktreeDir, runtime: agentruntime.RuntimeDocker,
	})
	transcript := f.writeClaudeTranscript(t, worktreeDir, "cccccccc-0000-4000-8000-000000000001")

	if err := svc.DeleteTask(context.Background(), "quick-docker"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	assertPathExists(t, transcript, true)
}

func TestDeleteQuickChatWithoutHistorySucceeds(t *testing.T) {
	svc, _, repo := createTestService(t)
	newAgentHistoryFixture(t, svc)
	seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-missing", ephemeral: true, agentName: "claude-acp", resumeToken: "dddddddd-0000-4000-8000-000000000001",
	})
	seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-no-token", ephemeral: true, agentName: "codex-acp",
	})

	for _, taskID := range []string{"quick-missing", "quick-no-token"} {
		if err := svc.DeleteTask(context.Background(), taskID); err != nil {
			t.Fatalf("DeleteTask(%s): %v", taskID, err)
		}
		waitForCleanupDone(t, svc)
		if _, err := repo.GetTask(context.Background(), taskID); err == nil {
			t.Fatalf("task %s should be deleted", taskID)
		}
	}
}

func TestDeleteQuickChatHistoryRemovalFailureDoesNotFailDeletion(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permissions do not restrict root")
	}
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	sessionID := seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-locked", ephemeral: true, agentName: "claude-acp", resumeToken: "eeeeeeee-0000-4000-8000-000000000001",
	})
	transcript := f.writeClaudeTranscript(t, filepath.Join(f.quickChatDir, sessionID), "eeeeeeee-0000-4000-8000-000000000001")
	project := filepath.Dir(transcript)
	if err := os.Chmod(project, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(project, 0o755) })

	if err := svc.DeleteTask(context.Background(), "quick-locked"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	if _, err := repo.GetTask(context.Background(), "quick-locked"); err == nil {
		t.Fatal("task should be deleted despite history removal failure")
	}
	assertPathExists(t, transcript, true)
}

func TestQuickChatExpirationRemovesAgentHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	sessionID := seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-expired-history", ephemeral: true, agentName: "claude-acp",
		resumeToken: "ffffffff-0000-4000-8000-000000000001", updatedAt: now.Add(-8 * 24 * time.Hour),
	})
	transcript := f.writeClaudeTranscript(t, filepath.Join(f.quickChatDir, sessionID), "ffffffff-0000-4000-8000-000000000001")

	svc.runQuickChatExpiration(context.Background(), now)
	waitForCleanupDone(t, svc)

	if _, err := repo.GetTask(context.Background(), "quick-expired-history"); err == nil {
		t.Fatal("expired quick chat should be deleted")
	}
	assertPathExists(t, transcript, false)
	assertPathExists(t, filepath.Dir(transcript), false)
}

// The Quick Chat tab close deletes through the task-tree cascade.
func TestDeleteQuickChatTreeRemovesItsAgentHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	claudeSession := seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-tree-claude", ephemeral: true, agentName: "claude-acp", resumeToken: "abababab-0000-4000-8000-000000000001",
	})
	claudeTranscript := f.writeClaudeTranscript(t, filepath.Join(f.quickChatDir, claudeSession), "abababab-0000-4000-8000-000000000001")
	codexSession := seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-tree-codex", ephemeral: true, agentName: "codex-acp", resumeToken: "01a109d5-1957-7462-9235-e91c49e4553a",
	})
	codexRollout := f.writeCodexRollout(t, filepath.Join(f.quickChatDir, codexSession), "01a109d5-1957-7462-9235-e91c49e4553a")
	normalWorktree := "/home/u/.kandev/tasks/normal-tree_abc/repo"
	seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "normal-tree", workflowID: "wf-tree", agentName: "claude-acp",
		resumeToken: "abababab-0000-4000-8000-000000000002", worktree: normalWorktree,
	})
	normalTranscript := f.writeClaudeTranscript(t, normalWorktree, "abababab-0000-4000-8000-000000000002")

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskEventPublisher(svc)
	handoff.SetTaskResourceCleaner(svc)
	for _, taskID := range []string{"quick-tree-claude", "quick-tree-codex", "normal-tree"} {
		if _, err := handoff.DeleteTaskTree(context.Background(), taskID, true); err != nil {
			t.Fatalf("DeleteTaskTree(%s): %v", taskID, err)
		}
		waitForCleanupDone(t, svc)
	}

	assertPathExists(t, claudeTranscript, false)
	assertPathExists(t, codexRollout, false)
	assertPathExists(t, normalTranscript, true)
}
