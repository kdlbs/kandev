package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSessionHasPendingClarification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if svc.sessionHasPendingClarification(ctx, "s1") {
		t.Fatal("expected no pending clarification")
	}

	now := time.Now().UTC()
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskSessionID: "s1", TaskID: "t1", StartedAt: now}))
	requireNoError(t, repo.CreateMessage(ctx, &models.Message{
		ID:            "clarify-1",
		TaskSessionID: "s1",
		TaskID:        "t1",
		TurnID:        "turn-1",
		AuthorType:    models.MessageAuthorAgent,
		Type:          "clarification_request",
		Content:       "Q?",
		CreatedAt:     now,
		Metadata: map[string]interface{}{
			"pending_id": "pending-1",
			"status":     "pending",
		},
	}))

	if !svc.sessionHasPendingClarification(ctx, "s1") {
		t.Fatal("expected pending clarification")
	}
}

// TestSessionHasPendingClarification_G5_AbsentOrUnrecognizedStatusDefers proves
// the guard's status predicate widened to D3's effective-pending form (spec
// G5): a clarification_request message with no metadata.status key, and one
// with an unrecognized status value, both still count as pending and defer
// turn-complete.
func TestSessionHasPendingClarification_G5_AbsentOrUnrecognizedStatusDefers(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]interface{}
	}{
		{"absent status key", map[string]interface{}{"pending_id": "pending-absent"}},
		{"unrecognized status value", map[string]interface{}{"pending_id": "pending-unrecognized", "status": "bogus"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

			now := time.Now().UTC()
			requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskSessionID: "s1", TaskID: "t1", StartedAt: now}))
			requireNoError(t, repo.CreateMessage(ctx, &models.Message{
				ID: "clarify-1", TaskSessionID: "s1", TaskID: "t1", TurnID: "turn-1",
				AuthorType: models.MessageAuthorAgent, Type: "clarification_request",
				Content: "Q?", CreatedAt: now, Metadata: tc.metadata,
			}))

			if !svc.sessionHasPendingClarification(ctx, "s1") {
				t.Fatal("expected the widened predicate to still count this message as pending")
			}
		})
	}
}

// TestSessionHasPendingClarification_D4a_TerminalStatusesDoNotDeferWithoutResolutionRow
// is the D4a regression the spec calls out as most important: a bundle whose
// messages are all terminal (answered/rejected/cancelled/expired) but has no
// clarification_resolutions row — the pre-upgrade legacy state — must not
// block turn-complete. Without D4a conjunct 2, a message with a recognized
// terminal status still correctly excludes itself; this proves G5's widening
// did not accidentally start counting terminal statuses as pending too.
func TestSessionHasPendingClarification_D4a_TerminalStatusesDoNotDeferWithoutResolutionRow(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	now := time.Now().UTC()
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskSessionID: "s1", TaskID: "t1", StartedAt: now}))
	for i, status := range []string{"answered", "rejected", "cancelled", "expired"} {
		requireNoError(t, repo.CreateMessage(ctx, &models.Message{
			ID: "clarify-" + status, TaskSessionID: "s1", TaskID: "t1", TurnID: "turn-1",
			AuthorType: models.MessageAuthorAgent, Type: "clarification_request",
			Content: "Q?", CreatedAt: now.Add(time.Duration(i) * time.Second),
			Metadata: map[string]interface{}{"pending_id": "pending-" + status, "status": status},
		}))
	}

	if svc.sessionHasPendingClarification(ctx, "s1") {
		t.Fatal("all-terminal legacy bundle with no resolution row must not block turn-complete")
	}
}

// TestTurnCompleteBlockedByUserInput_G1_ClaimedBundleDoesNotDefer proves the
// D4a conjunct-1 exclusion (spec G1/G2): once a clarification_resolutions row
// exists for a bundle, turnCompleteBlockedByUserInput no longer defers, even
// though the bundle's own message metadata still reads "pending" (the R5
// partial-application / claim-then-crash state). Without this exclusion the
// session would wedge forever, since re-answering (R2) and cancelling (X1)
// are both no-ops against an already-resolved bundle.
func TestTurnCompleteBlockedByUserInput_G1_ClaimedBundleDoesNotDefer(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	claimed, _, err := repo.InsertClarificationResolution(ctx, &models.ClarificationResolution{
		PendingID:  "pending-s1",
		SessionID:  "s1",
		TaskID:     "t1",
		Status:     models.ClarificationResolutionStatusAnswered,
		Response:   `{"pending_id":"pending-s1","answers":[],"rejected":false,"reject_reason":""}`,
		Resume:     "resumed",
		ResolvedBy: "user-1",
		Source:     models.ClarificationResolutionSourceWeb,
		ResolvedAt: time.Now().UTC(),
	})
	requireNoError(t, err)
	if !claimed {
		t.Fatal("InsertClarificationResolution did not claim the bundle")
	}

	session, err := repo.GetTaskSession(ctx, "s1")
	requireNoError(t, err)
	if svc.turnCompleteBlockedByUserInput(ctx, "t1", "s1", session) {
		t.Fatal("a resolved bundle must not block turn-complete despite stale pending message metadata")
	}
}

func seedPendingClarificationMessage(t *testing.T, repo interface {
	CreateTurn(ctx context.Context, turn *models.Turn) error
	CreateMessage(ctx context.Context, message *models.Message) error
}, taskID, sessionID string) {
	t.Helper()
	now := time.Now().UTC()
	turnID := "turn-clarification-" + sessionID
	requireNoError(t, repo.CreateTurn(context.Background(), &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
	}))
	requireNoError(t, repo.CreateMessage(context.Background(), &models.Message{
		ID:            "clarification-" + sessionID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          "clarification_request",
		Content:       "Which approach?",
		CreatedAt:     now,
		Metadata: map[string]interface{}{
			"pending_id":  "pending-" + sessionID,
			"question_id": "q1",
			"status":      "pending",
		},
	}))
}
