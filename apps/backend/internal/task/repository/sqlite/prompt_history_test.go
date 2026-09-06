package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type promptHistoryReader interface {
	HasUserPromptHistory(context.Context, string) (bool, error)
}

type promptHistoryClaimer interface {
	ClaimInitialPromptFallback(context.Context, string) (bool, error)
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

	claimed, err := claimer.ClaimInitialPromptFallback(ctx, "session-claim-first")
	if err != nil || !claimed {
		t.Fatalf("first fallback claim = %t, %v; want claimed", claimed, err)
	}
	if claimed, err := claimer.ClaimInitialPromptFallback(ctx, "session-claim-first"); err != nil || claimed {
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
	if claimed, err := claimer.ClaimInitialPromptFallback(ctx, "session-direct-first"); err != nil || claimed {
		t.Fatalf("direct-first fallback claim = %t, %v; want rejected", claimed, err)
	}

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, claimErr := claimer.ClaimInitialPromptFallback(ctx, "session-claim-race")
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
	claimed, err := claimer.ClaimInitialPromptFallback(ctx, "session-session-reuse")
	if err != nil || !claimed {
		t.Fatalf("initial fallback claim = %t, %v; want claimed", claimed, err)
	}
	if err := repo.DeleteTaskSession(ctx, "session-session-reuse"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	seedForMsgTest(t, repo, "task-session-reuse", "session-session-reuse", "turn-session-reuse-new")
	hasHistory, err := repo.HasUserPromptHistory(ctx, "session-session-reuse")
	if err != nil {
		t.Fatalf("read reused session history: %v", err)
	}
	if hasHistory {
		t.Fatal("reused session inherited prompt history from deleted session")
	}
}
