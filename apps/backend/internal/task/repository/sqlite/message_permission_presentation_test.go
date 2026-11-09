package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func createPermissionPresentationMessage(
	t *testing.T,
	repo *Repository,
	id, taskID, sessionID, turnID, pendingID string,
	presentation models.PermissionPresentation,
	status models.PermissionStatus,
	createdAt time.Time,
) {
	t.Helper()
	metadata := map[string]interface{}{
		"request_id": "request-" + id,
		"pending_id": pendingID,
		"options": []interface{}{
			map[string]interface{}{"option_id": "allow", "name": "Allow", "kind": "allow_once"},
		},
	}
	if presentation != "" {
		metadata[models.PermissionPresentationMetadataKey] = string(presentation)
	}
	if status != "" {
		metadata["status"] = string(status)
	}
	createInteractionMessage(t, repo, id, taskID, sessionID, turnID,
		models.MessageTypePermissionRequest, metadata, createdAt)
}

func pendingPermissionSessionIDs(t *testing.T, repo *Repository, sessionIDs []string) map[string]bool {
	t.Helper()
	actions, err := repo.GetPendingActionsBySessionIDs(context.Background(), sessionIDs)
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	out := make(map[string]bool, len(actions))
	for sessionID, action := range actions {
		if action == models.TaskPendingActionPermission {
			out[sessionID] = true
		}
	}
	return out
}

func pendingPermissionInteractionIDs(t *testing.T, repo *Repository, sessionIDs []string) []string {
	t.Helper()
	rows, err := repo.ListPendingInteractions(context.Background(), models.PendingInteractionFilter{
		SessionIDs: sessionIDs,
		Kinds:      []string{string(models.InteractionKindPermission)},
	})
	if err != nil {
		t.Fatalf("ListPendingInteractions: %v", err)
	}
	return interactionMessageIDs(rows)
}

func TestPendingPermissionProjectionsSkipAutomaticAndUnavailable(t *testing.T) {
	repo := newRepoForSessionTests(t)
	base := time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-auto", "session-auto")
	createPendingActionTurn(t, repo, "task-auto", "session-auto", "turn-auto", base, base)
	createPermissionPresentationMessage(t, repo, "auto-1", "task-auto", "session-auto", "turn-auto",
		"pending-auto", models.PermissionPresentationAutomatic, models.PermissionStatusPending, base)

	seedPendingActionSession(t, repo, "task-unavailable", "session-unavailable")
	createPendingActionTurn(t, repo, "task-unavailable", "session-unavailable", "turn-unavailable", base, base)
	createPermissionPresentationMessage(t, repo, "unavailable-1", "task-unavailable", "session-unavailable",
		"turn-unavailable", "pending-unavailable", models.PermissionPresentationUnavailable,
		models.PermissionStatusPending, base)

	seedPendingActionSession(t, repo, "task-manual", "session-manual")
	createPendingActionTurn(t, repo, "task-manual", "session-manual", "turn-manual", base, base)
	createPermissionPresentationMessage(t, repo, "manual-1", "task-manual", "session-manual", "turn-manual",
		"pending-manual", "", models.PermissionStatusPending, base)

	sessionIDs := []string{"session-auto", "session-unavailable", "session-manual"}
	pending := pendingPermissionSessionIDs(t, repo, sessionIDs)
	if !pending["session-manual"] || pending["session-auto"] || pending["session-unavailable"] {
		t.Fatalf("pending action projection = %#v, want only session-manual", pending)
	}
	if got := pendingPermissionInteractionIDs(t, repo, sessionIDs); len(got) != 1 || got[0] != "manual-1" {
		t.Fatalf("pending interactions = %v, want only manual-1", got)
	}
}

// TestPendingPermissionProjectionRanksNewestBeforeClassification pins the
// ordering rule: a newer automatic row supersedes an older pending manual row
// instead of letting the manual row become current input again.
func TestPendingPermissionProjectionRanksNewestBeforeClassification(t *testing.T) {
	repo := newRepoForSessionTests(t)
	base := time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-ranked", "session-ranked")
	createPendingActionTurn(t, repo, "task-ranked", "session-ranked", "turn-ranked", base, base)
	createPermissionPresentationMessage(t, repo, "older-manual", "task-ranked", "session-ranked", "turn-ranked",
		"pending-older", models.PermissionPresentationInteractive, models.PermissionStatusPending, base)
	createPermissionPresentationMessage(t, repo, "newer-auto", "task-ranked", "session-ranked", "turn-ranked",
		"pending-newer", models.PermissionPresentationAutomatic, models.PermissionStatusPending, base.Add(time.Second))

	if got := pendingPermissionSessionIDs(t, repo, []string{"session-ranked"}); len(got) != 0 {
		t.Fatalf("pending action projection = %#v, want none", got)
	}
	if got := pendingPermissionInteractionIDs(t, repo, []string{"session-ranked"}); len(got) != 0 {
		t.Fatalf("pending interactions = %v, want none", got)
	}
}

func TestAutomaticOnlyClaimRejectsPromotedFallback(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-claim", "session-claim")
	createPendingActionTurn(t, repo, "task-claim", "session-claim", "turn-claim", base, base)
	createPermissionPresentationMessage(t, repo, "claim-1", "task-claim", "session-claim", "turn-claim",
		"pending-claim", models.PermissionPresentationAutomatic, models.PermissionStatusPending, base)
	if _, updated, err := repo.UpdatePermissionPresentation(ctx, models.PermissionPresentationUpdate{
		TaskID: "task-claim", SessionID: "session-claim", RequestID: "request-claim-1",
		PendingID: "pending-claim", Presentation: models.PermissionPresentationInteractive,
		RequirePresentation: models.PermissionPresentationAutomatic, RequireUnclaimed: true,
	}); err != nil || !updated {
		t.Fatalf("promotion updated=%v err=%v", updated, err)
	}

	claim := models.PermissionResolutionClaimRequest{
		TaskID: "task-claim", SessionID: "session-claim", AutomaticOnly: true,
		Audit: models.PermissionResolutionAudit{
			ClaimID: "claim-auto", ActorKind: models.PermissionActorAutomation,
			Source: models.PermissionSourceAutoApprove, RequestID: "request-claim-1",
			PendingID: "pending-claim", OptionID: "allow", OptionKind: "allow_once",
		},
	}
	result, err := repo.ClaimPermissionResolution(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome == models.PermissionClaimed {
		t.Fatal("an automatic-only claim must not consume an interactive fallback")
	}

	// A manual resolver is still allowed to claim the same row.
	claim.AutomaticOnly = false
	claim.Audit.ClaimID = "claim-manual"
	result, err = repo.ClaimPermissionResolution(ctx, claim)
	if err != nil || result.Outcome != models.PermissionClaimed {
		t.Fatalf("manual claim = %+v err=%v", result, err)
	}
}

func TestUpdatePermissionPresentationMatchesExactIdentityAndClaim(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-update", "session-update")
	createPendingActionTurn(t, repo, "task-update", "session-update", "turn-update", base, base)
	createPermissionPresentationMessage(t, repo, "update-1", "task-update", "session-update", "turn-update",
		"pending-update", models.PermissionPresentationAutomatic, models.PermissionStatusPending, base)

	_, updated, err := repo.UpdatePermissionPresentation(ctx, models.PermissionPresentationUpdate{
		TaskID: "task-update", SessionID: "session-update", RequestID: "request-wrong",
		PendingID: "pending-update", Presentation: models.PermissionPresentationInteractive,
		RequirePresentation: models.PermissionPresentationAutomatic, RequireUnclaimed: true,
	})
	if err != nil || updated {
		t.Fatalf("mismatched request identity updated=%v err=%v", updated, err)
	}

	message, updated, err := repo.UpdatePermissionPresentation(ctx, models.PermissionPresentationUpdate{
		TaskID: "task-update", SessionID: "session-update", RequestID: "request-update-1",
		PendingID: "pending-update", Presentation: models.PermissionPresentationInteractive,
		RequirePresentation: models.PermissionPresentationAutomatic, RequireUnclaimed: true,
	})
	if err != nil || !updated || message == nil {
		t.Fatalf("promotion message=%+v updated=%v err=%v", message, updated, err)
	}
	if got := message.Metadata[models.PermissionPresentationMetadataKey]; got != string(models.PermissionPresentationInteractive) {
		t.Fatalf("permission_presentation = %#v, want interactive", got)
	}

	if _, err := repo.ClaimPermissionResolution(ctx, models.PermissionResolutionClaimRequest{
		TaskID: "task-update", SessionID: "session-update",
		Audit: models.PermissionResolutionAudit{
			ClaimID: "claim-update", ActorKind: models.PermissionActorAutomation,
			Source: models.PermissionSourceAutoApprove, RequestID: "request-update-1",
			PendingID: "pending-update", OptionID: "allow", OptionKind: "allow_once",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, updated, err := repo.UpdatePermissionPresentation(ctx, models.PermissionPresentationUpdate{
		TaskID: "task-update", SessionID: "session-update", RequestID: "request-update-1",
		PendingID: "pending-update", Presentation: models.PermissionPresentationUnavailable,
		RequirePresentation: models.PermissionPresentationInteractive, RequireUnclaimed: true,
	}); err != nil || updated {
		t.Fatalf("claimed row updated=%v err=%v", updated, err)
	}
}
