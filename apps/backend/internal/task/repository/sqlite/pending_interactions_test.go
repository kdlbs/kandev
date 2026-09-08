package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func createInteractionMessage(
	t *testing.T,
	repo *Repository,
	id, taskID, sessionID, turnID string,
	msgType models.MessageType,
	metadata map[string]interface{},
	createdAt time.Time,
) {
	t.Helper()
	if err := repo.CreateMessage(context.Background(), &models.Message{
		ID:            id,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       id,
		Type:          msgType,
		Metadata:      metadata,
		CreatedAt:     createdAt,
	}); err != nil {
		t.Fatalf("CreateMessage(%s): %v", id, err)
	}
}

func interactionMessageIDs(messages []*models.Message) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		out = append(out, message.ID)
	}
	return out
}

func TestListPendingInteractionsReturnsBothKinds(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-perm", "session-perm")
	createPendingActionTurn(t, repo, "task-perm", "session-perm", "turn-perm", base, base)
	createInteractionMessage(t, repo, "perm-1", "task-perm", "session-perm", "turn-perm",
		models.MessageTypePermissionRequest, map[string]interface{}{
			"pending_id":   "pending-perm",
			"tool_call_id": "call-1",
			"options": []interface{}{
				map[string]interface{}{"option_id": "allow", "name": "Allow", "kind": "allow_once"},
			},
		}, base)

	seedPendingActionSession(t, repo, "task-clar", "session-clar")
	createPendingActionTurn(t, repo, "task-clar", "session-clar", "turn-clar", base, base)
	createClarificationBundleMessage(t, repo, "clar-1", "task-clar", "session-clar", "turn-clar", "pending-clar", "q1", base)
	createClarificationBundleMessage(t, repo, "clar-2", "task-clar", "session-clar", "turn-clar", "pending-clar", "q2", base.Add(time.Second))

	got, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{})
	if err != nil {
		t.Fatalf("ListPendingInteractions: %v", err)
	}
	want := map[string]bool{"perm-1": true, "clar-1": true, "clar-2": true}
	if len(got) != len(want) {
		t.Fatalf("pending interactions = %v, want %v", interactionMessageIDs(got), want)
	}
	for _, message := range got {
		if !want[message.ID] {
			t.Fatalf("unexpected pending interaction %q in %v", message.ID, interactionMessageIDs(got))
		}
	}
}

// TestListPendingInteractionsAgreesWithPendingActionProjection pins the whole
// point of the durable read: it must reach exactly the same verdict as the
// compact pending-action projection the native list surfaces render. The
// approved-permission case is the concrete false positive reported against
// Kandev 0.90.0 — the turn completed, the only permission row is durably
// approved, and the session snapshot still reads WAITING_FOR_INPUT. A
// state-only consumer claims attention is owed; both projections must not.
func TestListPendingInteractionsAgreesWithPendingActionProjection(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)

	// Resolved: approved permission on a session still reading WAITING_FOR_INPUT.
	seedPendingActionSession(t, repo, "task-approved", "session-approved")
	createPendingActionTurn(t, repo, "task-approved", "session-approved", "turn-approved", base, base)
	createInteractionMessage(t, repo, "perm-approved", "task-approved", "session-approved", "turn-approved",
		models.MessageTypePermissionRequest, map[string]interface{}{
			"pending_id": "pending-approved",
			"status":     string(models.InteractionStatusApproved),
		}, base)

	// Genuinely pending: an unanswered permission on the current turn.
	seedPendingActionSession(t, repo, "task-owed", "session-owed")
	createPendingActionTurn(t, repo, "task-owed", "session-owed", "turn-owed", base, base)
	createInteractionMessage(t, repo, "perm-owed", "task-owed", "session-owed", "turn-owed",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-owed"}, base)

	sessionIDs := []string{"session-approved", "session-owed"}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, sessionIDs)
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	interactions, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{SessionIDs: sessionIDs})
	if err != nil {
		t.Fatalf("ListPendingInteractions: %v", err)
	}

	bySession := map[string]bool{}
	for _, message := range interactions {
		bySession[message.TaskSessionID] = true
	}
	for _, sessionID := range sessionIDs {
		_, projectionSaysPending := actions[sessionID]
		if projectionSaysPending != bySession[sessionID] {
			t.Fatalf("session %s: pending-action projection=%v, interaction list=%v (must agree)",
				sessionID, projectionSaysPending, bySession[sessionID])
		}
	}
	if bySession["session-approved"] {
		t.Fatal("approved permission reported as an owed interaction")
	}
	if !bySession["session-owed"] {
		t.Fatal("unanswered permission not reported as an owed interaction")
	}
}

func TestListPendingInteractionsExcludesSupersededTurnAndTerminalSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 20, 11, 0, 0, 0, time.UTC)

	// A clarification stranded on a superseded turn.
	seedPendingActionSession(t, repo, "task-old", "session-old")
	createPendingActionTurn(t, repo, "task-old", "session-old", "turn-old", base, base)
	createClarificationBundleMessage(t, repo, "clar-old", "task-old", "session-old", "turn-old", "pending-old", "q1", base)
	createPendingActionTurn(t, repo, "task-old", "session-old", "turn-new", base.Add(time.Minute), base.Add(time.Minute))
	createPendingActionMessage(t, repo, "ordinary-new", "task-old", "session-old", "turn-new",
		models.MessageTypeMessage, "<missing>", base.Add(time.Minute))

	// A pending clarification on a session that has since gone terminal.
	seedPendingActionSession(t, repo, "task-done", "session-done")
	createPendingActionTurn(t, repo, "task-done", "session-done", "turn-done", base, base)
	createClarificationBundleMessage(t, repo, "clar-done", "task-done", "session-done", "turn-done", "pending-done", "q1", base)
	session, err := repo.GetTaskSession(ctx, "session-done")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.State = models.TaskSessionStateCompleted
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	got, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{})
	if err != nil {
		t.Fatalf("ListPendingInteractions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("pending interactions = %v, want none", interactionMessageIDs(got))
	}
}

// TestListPendingInteractionsReportsOnlyNewestPermissionOfTurn mirrors the
// pending-action projection's latest-permission rule: an older permission row
// in the same turn is not independently answerable, so surfacing it would
// hand a plugin an id whose response the agent has already moved past.
func TestListPendingInteractionsReportsOnlyNewestPermissionOfTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-multi", "session-multi")
	createPendingActionTurn(t, repo, "task-multi", "session-multi", "turn-multi", base, base)
	createInteractionMessage(t, repo, "perm-older", "task-multi", "session-multi", "turn-multi",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-older"}, base)
	createInteractionMessage(t, repo, "perm-newer", "task-multi", "session-multi", "turn-multi",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-newer"}, base.Add(time.Second))

	got, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{})
	if err != nil {
		t.Fatalf("ListPendingInteractions: %v", err)
	}
	if len(got) != 1 || got[0].ID != "perm-newer" {
		t.Fatalf("pending interactions = %v, want [perm-newer]", interactionMessageIDs(got))
	}
}

func TestListPendingInteractionsFilters(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 20, 13, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-a", "session-a")
	createPendingActionTurn(t, repo, "task-a", "session-a", "turn-a", base, base)
	createInteractionMessage(t, repo, "perm-a", "task-a", "session-a", "turn-a",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-a"}, base)

	seedPendingActionSession(t, repo, "task-b", "session-b")
	createPendingActionTurn(t, repo, "task-b", "session-b", "turn-b", base, base)
	createClarificationBundleMessage(t, repo, "clar-b", "task-b", "session-b", "turn-b", "pending-b", "q1", base)

	cases := []struct {
		name   string
		filter models.PendingInteractionFilter
		want   []string
	}{
		{"by session", models.PendingInteractionFilter{SessionIDs: []string{"session-a"}}, []string{"perm-a"}},
		{"by task", models.PendingInteractionFilter{TaskIDs: []string{"task-b"}}, []string{"clar-b"}},
		{"by kind", models.PendingInteractionFilter{
			Kinds: []string{string(models.InteractionKindClarification)},
		}, []string{"clar-b"}},
		{"session and task must both match", models.PendingInteractionFilter{
			SessionIDs: []string{"session-a"}, TaskIDs: []string{"task-b"},
		}, nil},
		{"unknown kind matches nothing", models.PendingInteractionFilter{Kinds: []string{"nope"}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.ListPendingInteractions(ctx, tc.filter)
			if err != nil {
				t.Fatalf("ListPendingInteractions: %v", err)
			}
			ids := interactionMessageIDs(got)
			if len(ids) != len(tc.want) {
				t.Fatalf("ids = %v, want %v", ids, tc.want)
			}
			for i := range ids {
				if ids[i] != tc.want[i] {
					t.Fatalf("ids = %v, want %v", ids, tc.want)
				}
			}
		})
	}
}

// TestListPendingInteractionsReturnsWholeBundleWhenPartiallyAnswered pins the
// bundle contract: the agent stays blocked until EVERY question of a
// clarification is answered, so a bundle with one answered and one pending
// question must come back whole. Returning only the pending row would hand a
// caller a question set smaller than the answer path requires (the live
// clarification request still expects one answer per original question), and
// would disagree with GetInteraction, which resolves the bundle by pending id.
func TestListPendingInteractionsReturnsWholeBundleWhenPartiallyAnswered(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 22, 14, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-partial", "session-partial")
	createPendingActionTurn(t, repo, "task-partial", "session-partial", "turn-partial", base, base)
	createInteractionMessage(t, repo, "clar-answered", "task-partial", "session-partial", "turn-partial",
		models.MessageTypeClarificationRequest, map[string]interface{}{
			"pending_id":     "pending-partial",
			"question_id":    "q1",
			"question_index": 0,
			"status":         string(models.InteractionStatusAnswered),
		}, base)
	createInteractionMessage(t, repo, "clar-pending", "task-partial", "session-partial", "turn-partial",
		models.MessageTypeClarificationRequest, map[string]interface{}{
			"pending_id":     "pending-partial",
			"question_id":    "q2",
			"question_index": 1,
			"status":         string(models.InteractionStatusPending),
		}, base.Add(time.Second))

	// A fully answered bundle on the same turn must stay out entirely.
	createInteractionMessage(t, repo, "clar-done", "task-partial", "session-partial", "turn-partial",
		models.MessageTypeClarificationRequest, map[string]interface{}{
			"pending_id":     "pending-done",
			"question_id":    "q1",
			"question_index": 0,
			"status":         string(models.InteractionStatusAnswered),
		}, base.Add(2*time.Second))

	got, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{})
	if err != nil {
		t.Fatalf("ListPendingInteractions: %v", err)
	}
	ids := interactionMessageIDs(got)
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want the whole partially-answered bundle and nothing else", ids)
	}
	for _, want := range []string{"clar-answered", "clar-pending"} {
		found := false
		for _, id := range ids {
			if id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("ids = %v, missing %q from the pending bundle", ids, want)
		}
	}
}
