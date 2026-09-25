package sqlite

import (
	"context"
	"strings"
	"testing"
)

// TestListOrphanedMessagePayloadCandidatesReportsUnreferencedRowsOnly is
// this wave's coverage for the "stale payload" retention category (Task
// 06): a task_message_payloads row still referenced by a message must never
// be reported, even if another byte-identical (deduped) payload row that
// nothing references any longer exists alongside it.
func TestListOrphanedMessagePayloadCandidatesReportsUnreferencedRowsOnly(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-orphan", "sess-payload-orphan", "turn-1")

	referenced := newShellMessage("msg-orphan-referenced", "sess-payload-orphan", strings.Repeat("r", largeMessagePayloadThresholdBytes+1), "")
	referenced.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, referenced); err != nil {
		t.Fatalf("CreateMessage(referenced): %v", err)
	}

	toOrphan := newShellMessage("msg-orphan-deleted", "sess-payload-orphan", strings.Repeat("o", largeMessagePayloadThresholdBytes+1), "")
	toOrphan.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, toOrphan); err != nil {
		t.Fatalf("CreateMessage(toOrphan): %v", err)
	}
	orphanDigest := toOrphan.PayloadDigest
	if orphanDigest == "" {
		t.Fatal("expected toOrphan to externalize a payload digest")
	}

	// Delete the only message referencing orphanDigest so its payload row
	// becomes unreferenced, without touching the still-referenced payload
	// row's message.
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`DELETE FROM task_session_messages WHERE id = ?`), toOrphan.ID); err != nil {
		t.Fatalf("delete toOrphan message: %v", err)
	}

	candidates, err := repo.ListOrphanedMessagePayloadCandidates(ctx, 0)
	if err != nil {
		t.Fatalf("ListOrphanedMessagePayloadCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %+v, want exactly 1 orphaned payload", candidates)
	}
	if candidates[0].Digest != orphanDigest {
		t.Fatalf("candidate digest = %q, want %q", candidates[0].Digest, orphanDigest)
	}
	if candidates[0].CompressedSize <= 0 || candidates[0].UncompressedSize <= 0 {
		t.Fatalf("candidate sizes = %+v, want positive compressed/uncompressed sizes", candidates[0])
	}

	// The still-referenced payload's digest must never appear.
	for _, c := range candidates {
		if c.Digest == referenced.PayloadDigest {
			t.Fatalf("still-referenced digest %q reported as orphaned", referenced.PayloadDigest)
		}
	}
}

// TestListOrphanedMessagePayloadCandidatesIsNonDestructive confirms the
// selection is read-only: calling it does not remove or alter any payload
// row, matching the plan's "non-destructive until maintenance executes"
// constraint.
func TestListOrphanedMessagePayloadCandidatesIsNonDestructive(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-orphan-nd", "sess-payload-orphan-nd", "turn-1")

	msg := newShellMessage("msg-orphan-nd", "sess-payload-orphan-nd", strings.Repeat("n", largeMessagePayloadThresholdBytes+1), "")
	msg.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`DELETE FROM task_session_messages WHERE id = ?`), msg.ID); err != nil {
		t.Fatalf("delete message: %v", err)
	}

	before := countRows(t, repo, `SELECT COUNT(*) FROM task_message_payloads`)
	if _, err := repo.ListOrphanedMessagePayloadCandidates(ctx, 0); err != nil {
		t.Fatalf("ListOrphanedMessagePayloadCandidates: %v", err)
	}
	after := countRows(t, repo, `SELECT COUNT(*) FROM task_message_payloads`)
	if before != after {
		t.Fatalf("row count changed from %d to %d after a read-only candidate listing", before, after)
	}
	if before == 0 {
		t.Fatal("expected at least one payload row to exist for this test to be meaningful")
	}
}
