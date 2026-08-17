package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestDetachActiveClarificationMessagesClaimsOnlyCurrentPendingRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 15, 30, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-detach", "session-detach")
	createPendingActionTurn(t, repo, "task-detach", "session-detach", "turn-old", base, base)
	createClarificationBundleMessage(
		t, repo, "message-old", "task-detach", "session-detach", "turn-old",
		"pending-old", "q-old", base,
	)
	createPendingActionTurn(
		t, repo, "task-detach", "session-detach", "turn-current",
		base.Add(time.Minute), base.Add(time.Minute),
	)
	createClarificationBundleMessage(
		t, repo, "message-current", "task-detach", "session-detach", "turn-current",
		"pending-current", "q-current", base.Add(time.Minute),
	)
	createClarificationBundleMessage(
		t, repo, "message-terminal", "task-detach", "session-detach", "turn-current",
		"pending-terminal", "q-terminal", base.Add(time.Minute+time.Second),
	)
	setClarificationMessageMetadata(t, repo, "message-terminal", func(metadata map[string]interface{}) {
		metadata["status"] = "answered"
	})
	createClarificationBundleMessage(
		t, repo, "message-detached", "task-detach", "session-detach", "turn-current",
		"pending-detached", "q-detached", base.Add(time.Minute+2*time.Second),
	)
	setClarificationMessageMetadata(t, repo, "message-detached", func(metadata map[string]interface{}) {
		metadata["agent_disconnected"] = true
	})
	mutationTime := base.Add(2*time.Hour + 123*time.Millisecond)
	repo.clockNow = func() time.Time { return mutationTime }
	previousUpdatedAt := setMessageUpdatedAtBeforeMutation(t, repo, "message-current", mutationTime)

	updated, err := repo.DetachActiveClarificationMessagesBySessionID(ctx, "session-detach")
	if err != nil {
		t.Fatalf("DetachActiveClarificationMessagesBySessionID: %v", err)
	}
	if ids := messageIDs(updated); len(ids) != 1 || ids[0] != "message-current" {
		t.Fatalf("detached message IDs = %v, want only current pending row", ids)
	}
	if !updated[0].UpdatedAt.After(previousUpdatedAt) {
		t.Fatalf("detached updated_at = %v, want after prior Go timestamp %v", updated[0].UpdatedAt, previousUpdatedAt)
	}
	if !updated[0].UpdatedAt.Equal(mutationTime) {
		t.Fatalf("detached updated_at = %v, want injected mutation time %v", updated[0].UpdatedAt, mutationTime)
	}
	current, err := repo.GetMessage(ctx, "message-current")
	if err != nil {
		t.Fatalf("GetMessage(current): %v", err)
	}
	if detached, _ := current.Metadata["agent_disconnected"].(bool); !detached {
		t.Fatalf("current message metadata = %#v, want agent_disconnected=true", current.Metadata)
	}
	old, err := repo.GetMessage(ctx, "message-old")
	if err != nil {
		t.Fatalf("GetMessage(old): %v", err)
	}
	if _, detached := old.Metadata["agent_disconnected"]; detached {
		t.Fatalf("superseded message was detached: %#v", old.Metadata)
	}
	repeated, err := repo.DetachActiveClarificationMessagesBySessionID(ctx, "session-detach")
	if err != nil {
		t.Fatalf("repeated detach: %v", err)
	}
	if len(repeated) != 0 {
		t.Fatalf("repeated detach changed rows: %v", messageIDs(repeated))
	}
}

func TestDetachActiveClarificationMessagesTreatsTruthyStringsAsDetached(t *testing.T) {
	for _, flag := range []string{"true", "1"} {
		t.Run(flag, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			createdAt := time.Date(2026, time.August, 15, 15, 40, 0, 0, time.UTC)
			seedPendingActionSession(t, repo, "task-string-detach", "session-string-detach")
			createPendingActionTurn(
				t, repo, "task-string-detach", "session-string-detach", "turn-string-detach",
				createdAt, createdAt,
			)
			createClarificationBundleMessage(
				t, repo, "message-string-detach", "task-string-detach", "session-string-detach",
				"turn-string-detach", "pending-string-detach", "q-string-detach", createdAt,
			)
			setClarificationMessageMetadata(t, repo, "message-string-detach", func(metadata map[string]interface{}) {
				metadata["agent_disconnected"] = flag
			})

			updated, err := repo.DetachActiveClarificationMessagesBySessionID(
				ctx,
				"session-string-detach",
			)
			if err != nil {
				t.Fatalf("DetachActiveClarificationMessagesBySessionID: %v", err)
			}
			if len(updated) != 0 {
				t.Fatalf("detached message IDs = %v, want none", messageIDs(updated))
			}
		})
	}
}

func TestExpireActiveClarificationMessagesClaimsOnlyCurrentPendingRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 15, 45, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-expire", "session-expire")
	createPendingActionTurn(t, repo, "task-expire", "session-expire", "turn-old", base, base)
	createClarificationBundleMessage(
		t, repo, "message-old", "task-expire", "session-expire", "turn-old",
		"pending-old", "q-old", base,
	)
	createPendingActionTurn(
		t, repo, "task-expire", "session-expire", "turn-current",
		base.Add(time.Minute), base.Add(time.Minute),
	)
	createClarificationBundleMessage(
		t, repo, "message-current", "task-expire", "session-expire", "turn-current",
		"pending-current", "q-current", base.Add(time.Minute),
	)
	createClarificationBundleMessage(
		t, repo, "message-other-pending", "task-expire", "session-expire", "turn-current",
		"pending-other", "q-other", base.Add(time.Minute+500*time.Millisecond),
	)
	createClarificationBundleMessage(
		t, repo, "message-answered", "task-expire", "session-expire", "turn-current",
		"pending-answered", "q-answered", base.Add(time.Minute+time.Second),
	)
	setClarificationMessageMetadata(t, repo, "message-answered", func(metadata map[string]interface{}) {
		metadata["status"] = "answered"
	})
	mutationTime := base.Add(2*time.Hour + 456*time.Millisecond)
	repo.clockNow = func() time.Time { return mutationTime }
	previousUpdatedAt := setMessageUpdatedAtBeforeMutation(t, repo, "message-current", mutationTime)

	updated, err := repo.ExpireActiveClarificationBundle(ctx, "session-expire", "pending-current")
	if err != nil {
		t.Fatalf("ExpireActiveClarificationBundle: %v", err)
	}
	if ids := messageIDs(updated); len(ids) != 1 || ids[0] != "message-current" {
		t.Fatalf("expired message IDs = %v, want only current pending row", ids)
	}
	if !updated[0].UpdatedAt.After(previousUpdatedAt) {
		t.Fatalf("expired updated_at = %v, want after prior Go timestamp %v", updated[0].UpdatedAt, previousUpdatedAt)
	}
	if !updated[0].UpdatedAt.Equal(mutationTime) {
		t.Fatalf("expired updated_at = %v, want injected mutation time %v", updated[0].UpdatedAt, mutationTime)
	}
	current, err := repo.GetMessage(ctx, "message-current")
	if err != nil {
		t.Fatalf("GetMessage(current): %v", err)
	}
	if current.Metadata["status"] != "expired" || current.Metadata["agent_disconnected"] != true {
		t.Fatalf("current message metadata = %#v, want expired and disconnected", current.Metadata)
	}
	answered, err := repo.GetMessage(ctx, "message-answered")
	if err != nil {
		t.Fatalf("GetMessage(answered): %v", err)
	}
	if answered.Metadata["status"] != "answered" {
		t.Fatalf("answered message status = %v, want answered", answered.Metadata["status"])
	}
	other, err := repo.GetMessage(ctx, "message-other-pending")
	if err != nil {
		t.Fatalf("GetMessage(other pending): %v", err)
	}
	if other.Metadata["status"] != "pending" {
		t.Fatalf("other bundle status = %v, want pending", other.Metadata["status"])
	}
	old, err := repo.GetMessage(ctx, "message-old")
	if err != nil {
		t.Fatalf("GetMessage(old): %v", err)
	}
	if old.Metadata["status"] != "pending" {
		t.Fatalf("superseded message status = %v, want pending", old.Metadata["status"])
	}
	repeated, err := repo.ExpireActiveClarificationBundle(ctx, "session-expire", "pending-current")
	if err != nil {
		t.Fatalf("repeated expiry: %v", err)
	}
	if len(repeated) != 0 {
		t.Fatalf("repeated expiry changed rows: %v", messageIDs(repeated))
	}
}

// setMessageUpdatedAtBeforeMutation seeds the immediate predecessor to a
// controlled repository clock so timestamp ordering needs no wall-clock wait.
func setMessageUpdatedAtBeforeMutation(
	t *testing.T,
	repo *Repository,
	messageID string,
	mutationTime time.Time,
) time.Time {
	t.Helper()
	updatedAt := mutationTime.Add(-time.Nanosecond)
	if _, err := repo.db.Exec(
		repo.db.Rebind(`UPDATE task_session_messages SET updated_at = ? WHERE id = ?`),
		updatedAt,
		messageID,
	); err != nil {
		t.Fatalf("seed message updated_at: %v", err)
	}
	return updatedAt
}

func TestRestoreClarificationMessagesRechecksCurrentTurnAtUpdate(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 16, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-restore-race", "session-restore-race")
	createPendingActionTurn(
		t, repo, "task-restore-race", "session-restore-race", "turn-restore-race", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-restore-race", "task-restore-race", "session-restore-race",
		"turn-restore-race", "pending-restore-race", "q1", base,
	)
	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-restore-race",
		"answered",
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil || !claimed {
		t.Fatalf("complete before restore race: claimed=%v err=%v", claimed, err)
	}
	createPendingActionTurn(
		t, repo, "task-restore-race", "session-restore-race", "turn-successor",
		base.Add(time.Second), base.Add(time.Second),
	)

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	restoreErr := repo.restoreClarificationMessages(
		ctx,
		tx,
		repo.db.DriverName(),
		claimedMessages,
		"answered",
	)
	_ = tx.Rollback()
	if restoreErr == nil {
		t.Fatal("restore update accepted a bundle after a successor turn became current")
	}
	message, err := repo.GetMessage(ctx, "message-restore-race")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != "answered" {
		t.Fatalf("superseded terminal message status = %v, want answered", message.Metadata["status"])
	}
}

func setClarificationMessageMetadata(
	t *testing.T,
	repo *Repository,
	messageID string,
	update func(map[string]interface{}),
) {
	t.Helper()
	message, err := repo.GetMessage(context.Background(), messageID)
	if err != nil {
		t.Fatalf("GetMessage(%s): %v", messageID, err)
	}
	update(message.Metadata)
	if err := repo.UpdateMessage(context.Background(), message); err != nil {
		t.Fatalf("UpdateMessage(%s): %v", messageID, err)
	}
}
