package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type promptHistoryReader interface {
	HasUserPromptHistory(context.Context, string) (bool, error)
}

type promptHistoryClaimer interface {
	ClaimInitialPromptFallback(context.Context, string, string) (bool, error)
}

func promptHistoryIncarnation(t *testing.T, repo *Repository, ctx context.Context, sessionID string) string {
	t.Helper()
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session %q: %v", sessionID, err)
	}
	return session.QueueIncarnationID
}

func TestHasUserPromptHistory(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-history-a", "session-history-a", "turn-history-a")
	seedForMsgTest(t, repo, "task-history-b", "session-history-b", "turn-history-b")

	reader, ok := any(repo).(promptHistoryReader)
	if !ok {
		t.Fatal("repository does not expose HasUserPromptHistory")
	}

	hasHistory, err := reader.HasUserPromptHistory(ctx, "session-history-a")
	if err != nil {
		t.Fatalf("empty session history: %v", err)
	}
	if hasHistory {
		t.Fatal("empty session reports user prompt history")
	}

	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "history-agent",
		TaskSessionID: "session-history-a",
		TurnID:        "turn-history-a",
		AuthorType:    models.MessageAuthorAgent,
		Content:       "agent output",
	}); err != nil {
		t.Fatalf("create agent message: %v", err)
	}
	hasHistory, err = reader.HasUserPromptHistory(ctx, "session-history-a")
	if err != nil {
		t.Fatalf("agent-only session history: %v", err)
	}
	if hasHistory {
		t.Fatal("agent-only session reports user prompt history")
	}

	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "history-user",
		TaskSessionID: "session-history-a",
		TurnID:        "turn-history-a",
		AuthorType:    models.MessageAuthorUser,
		Content:       "first user prompt",
	}); err != nil {
		t.Fatalf("create user message: %v", err)
	}
	hasHistory, err = reader.HasUserPromptHistory(ctx, "session-history-a")
	if err != nil {
		t.Fatalf("session history after user message: %v", err)
	}
	if !hasHistory {
		t.Fatal("session with a user message reports no history")
	}

	if err := repo.DeleteMessage(ctx, "history-user"); err != nil {
		t.Fatalf("delete user message: %v", err)
	}
	hasHistory, err = reader.HasUserPromptHistory(ctx, "session-history-a")
	if err != nil {
		t.Fatalf("session history after deletion: %v", err)
	}
	if !hasHistory {
		t.Fatal("deleting a user message made prompt history eligible again")
	}

	hasHistory, err = reader.HasUserPromptHistory(ctx, "session-history-b")
	if err != nil {
		t.Fatalf("other session history: %v", err)
	}
	if hasHistory {
		t.Fatal("prompt history leaked between sessions")
	}
}

func TestClaimInitialPromptFallbackSerializesPromptAdmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-claim-first", "session-claim-first", "turn-claim-first")
	seedForMsgTest(t, repo, "task-direct-first", "session-direct-first", "turn-direct-first")
	seedForMsgTest(t, repo, "task-claim-race", "session-claim-race", "turn-claim-race")

	claimer, ok := any(repo).(promptHistoryClaimer)
	if !ok {
		t.Fatal("repository does not expose ClaimInitialPromptFallback")
	}
	claimIncarnation := promptHistoryIncarnation(t, repo, ctx, "session-claim-first")

	claimed, err := claimer.ClaimInitialPromptFallback(ctx, "session-claim-first", claimIncarnation)
	if err != nil || !claimed {
		t.Fatalf("first fallback claim = %t, %v; want claimed", claimed, err)
	}
	if claimed, err := claimer.ClaimInitialPromptFallback(ctx, "session-claim-first", claimIncarnation); err != nil || claimed {
		t.Fatalf("second fallback claim = %t, %v; want rejected", claimed, err)
	}
	claimedPrompt := &models.Message{
		ID:            "claimed-fallback-user",
		TaskID:        "task-claim-first",
		TaskSessionID: "session-claim-first",
		TurnID:        "turn-claim-first",
		AuthorType:    models.MessageAuthorUser,
		Content:       "claimed fallback",
	}
	if err := repo.CreateMessage(ctx, claimedPrompt); err != nil {
		t.Fatalf("create claimed fallback prompt: %v", err)
	}
	if claimedPrompt.PromptIndex != 1 {
		t.Fatalf("claimed fallback prompt index = %d, want 1", claimedPrompt.PromptIndex)
	}

	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "direct-first-user",
		TaskID:        "task-direct-first",
		TaskSessionID: "session-direct-first",
		TurnID:        "turn-direct-first",
		AuthorType:    models.MessageAuthorUser,
		Content:       "direct prompt",
	}); err != nil {
		t.Fatalf("create direct prompt: %v", err)
	}
	directIncarnation := promptHistoryIncarnation(t, repo, ctx, "session-direct-first")
	if claimed, err := claimer.ClaimInitialPromptFallback(ctx, "session-direct-first", directIncarnation); err != nil || claimed {
		t.Fatalf("direct-first fallback claim = %t, %v; want rejected", claimed, err)
	}

	raceIncarnation := promptHistoryIncarnation(t, repo, ctx, "session-claim-race")

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, claimErr := claimer.ClaimInitialPromptFallback(ctx, "session-claim-race", raceIncarnation)
			if claimErr != nil {
				t.Errorf("concurrent fallback claim: %v", claimErr)
				return
			}
			results <- claimed
		}()
	}
	wg.Wait()
	close(results)
	var claimedCount int
	for claimed := range results {
		if claimed {
			claimedCount++
		}
	}
	if claimedCount != 1 {
		t.Fatalf("concurrent fallback claims = %d, want exactly one winner", claimedCount)
	}
}

func TestDeleteTaskSessionRemovesPromptHistoryClaim(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-session-reuse", "session-session-reuse", "turn-session-reuse")

	claimer := any(repo).(promptHistoryClaimer)
	oldSession, err := repo.GetTaskSession(ctx, "session-session-reuse")
	if err != nil {
		t.Fatalf("read initial session: %v", err)
	}
	claimed, err := claimer.ClaimInitialPromptFallback(ctx, "session-session-reuse", oldSession.QueueIncarnationID)
	if err != nil || !claimed {
		t.Fatalf("initial fallback claim = %t, %v; want claimed", claimed, err)
	}
	if err := deleteTaskSessionForTest(t, repo, ctx, "session-session-reuse"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	seedForMsgTest(t, repo, "task-session-reuse", "session-session-reuse", "turn-session-reuse-new")
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?
	`), "replacement-session-incarnation", "session-session-reuse"); err != nil {
		t.Fatalf("set replacement session incarnation: %v", err)
	}
	newSession, err := repo.GetTaskSession(ctx, "session-session-reuse")
	if err != nil {
		t.Fatalf("read replacement session: %v", err)
	}
	if newSession.QueueIncarnationID == oldSession.QueueIncarnationID {
		t.Fatal("replacement session reused its predecessor's incarnation")
	}

	claimed, err = claimer.ClaimInitialPromptFallback(ctx, "session-session-reuse", oldSession.QueueIncarnationID)
	if err != nil {
		t.Fatalf("stale fallback claim: %v", err)
	}
	if claimed {
		t.Fatal("stale fallback claim reserved the replacement session's first-prompt slot")
	}
	hasHistory, err := repo.HasUserPromptHistory(ctx, "session-session-reuse")
	if err != nil {
		t.Fatalf("read reused session history: %v", err)
	}
	if hasHistory {
		t.Fatal("reused session inherited prompt history from deleted session")
	}
	claimed, err = claimer.ClaimInitialPromptFallback(ctx, "session-session-reuse", newSession.QueueIncarnationID)
	if err != nil || !claimed {
		t.Fatalf("replacement fallback claim = %t, %v; want claimed", claimed, err)
	}

}
func TestDeleteTaskClearsPromptSequenceBeforeSessionIDReuse(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-reuse-after-delete", "session-reuse-after-delete", "turn-before-delete")

	first := &models.Message{
		ID: "prompt-before-task-delete", TaskID: "task-reuse-after-delete",
		TaskSessionID: "session-reuse-after-delete", TurnID: "turn-before-delete",
		AuthorType: models.MessageAuthorUser, Content: "first prompt",
	}
	if err := repo.CreateMessage(ctx, first); err != nil {
		t.Fatalf("create first prompt: %v", err)
	}
	if first.PromptIndex != 1 {
		t.Fatalf("first prompt ordinal = %d, want 1", first.PromptIndex)
	}
	if err := repo.DeleteTask(ctx, "task-reuse-after-delete"); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	seedForMsgTest(t, repo, "task-reuse-after-delete", "session-reuse-after-delete", "turn-after-delete")
	hasHistory, err := repo.HasUserPromptHistory(ctx, "session-reuse-after-delete")
	if err != nil {
		t.Fatalf("read recreated session history: %v", err)
	}
	if hasHistory {
		t.Fatal("recreated session inherited prompt history from deleted task")
	}
	claimed, err := repo.ClaimInitialPromptFallback(ctx, "session-reuse-after-delete", promptHistoryIncarnation(t, repo, ctx, "session-reuse-after-delete"))
	if err != nil || !claimed {
		t.Fatalf("initial fallback claim after task delete = %t, %v; want claimed", claimed, err)
	}

	recreated := &models.Message{
		ID: "prompt-after-task-delete", TaskID: "task-reuse-after-delete",
		TaskSessionID: "session-reuse-after-delete", TurnID: "turn-after-delete",
		AuthorType: models.MessageAuthorUser, Content: "new first prompt",
	}
	if err := repo.CreateMessage(ctx, recreated); err != nil {
		t.Fatalf("create first prompt in recreated session: %v", err)
	}
	if recreated.PromptIndex != 1 {
		t.Fatalf("recreated session first prompt ordinal = %d, want 1", recreated.PromptIndex)
	}
}

func TestDeleteEphemeralTaskClearsPromptSequence(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	taskID, sessionID, turnID := "ephemeral-prompt-task", "ephemeral-prompt-session", "ephemeral-prompt-turn"
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE tasks SET is_ephemeral = 1 WHERE id = ?
	`), taskID); err != nil {
		t.Fatalf("mark task ephemeral: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_sessions SET agent_profile_id = ? WHERE id = ?
	`), "profile-to-delete", sessionID); err != nil {
		t.Fatalf("assign agent profile: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "ephemeral-prompt", TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorUser, Content: "prompt",
	}); err != nil {
		t.Fatalf("create user prompt: %v", err)
	}
	if count, err := repo.DeleteEphemeralTasksByAgentProfile(ctx, "profile-to-delete"); err != nil || count != 1 {
		t.Fatalf("delete ephemeral task = %d, %v; want one task", count, err)
	}
	seedForMsgTest(t, repo, taskID, sessionID, "ephemeral-prompt-turn-new")
	hasHistory, err := repo.HasUserPromptHistory(ctx, sessionID)
	if err != nil || hasHistory {
		t.Fatalf("recreated ephemeral session history = %t, %v; want empty", hasHistory, err)
	}
}

func TestDeleteExpiredQuickChatClearsPromptSequence(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	taskID, sessionID, turnID := "expired-prompt-task", "expired-prompt-session", "expired-prompt-turn"
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	cutoff := time.Now().UTC()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE tasks SET is_ephemeral = 1, updated_at = ? WHERE id = ?
	`), cutoff.Add(-time.Hour), taskID); err != nil {
		t.Fatalf("expire task: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "expired-prompt", TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorUser, Content: "prompt",
	}); err != nil {
		t.Fatalf("create user prompt: %v", err)
	}
	deleted, err := repo.DeleteExpiredQuickChatTask(ctx, taskID, cutoff)
	if err != nil || !deleted {
		t.Fatalf("delete expired quick chat = %t, %v; want deleted", deleted, err)
	}
	seedForMsgTest(t, repo, taskID, sessionID, "expired-prompt-turn-new")
	hasHistory, err := repo.HasUserPromptHistory(ctx, sessionID)
	if err != nil || hasHistory {
		t.Fatalf("recreated quick-chat session history = %t, %v; want empty", hasHistory, err)
	}
}

func TestDeleteWorkspaceCascadeClearsPromptSequence(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const workspaceID = "prompt-delete-workspace"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Prompt deletion"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	taskID, sessionID, turnID := "workspace-prompt-task", "workspace-prompt-session", "workspace-prompt-turn"
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE tasks SET workspace_id = ? WHERE id = ?
	`), workspaceID, taskID); err != nil {
		t.Fatalf("assign workspace: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "workspace-prompt", TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorUser, Content: "prompt",
	}); err != nil {
		t.Fatalf("create user prompt: %v", err)
	}
	if _, _, err := repo.DeleteWorkspaceCascade(ctx, workspaceID); err != nil {
		t.Fatalf("delete workspace cascade: %v", err)
	}
	seedForMsgTest(t, repo, taskID, sessionID, "workspace-prompt-turn-new")
	hasHistory, err := repo.HasUserPromptHistory(ctx, sessionID)
	if err != nil || hasHistory {
		t.Fatalf("recreated workspace session history = %t, %v; want empty", hasHistory, err)
	}
}
