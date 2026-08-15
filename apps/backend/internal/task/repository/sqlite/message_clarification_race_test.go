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

	updated, err := repo.DetachActiveClarificationMessagesBySessionID(ctx, "session-detach")
	if err != nil {
		t.Fatalf("DetachActiveClarificationMessagesBySessionID: %v", err)
	}
	if ids := messageIDs(updated); len(ids) != 1 || ids[0] != "message-current" {
		t.Fatalf("detached message IDs = %v, want only current pending row", ids)
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
