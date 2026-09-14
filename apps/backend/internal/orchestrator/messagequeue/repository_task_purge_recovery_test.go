package messagequeue

import (
	"context"
	"testing"
)

func TestSQLiteTaskPurgeRemovesOrdinaryDispatchClaim(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := insertTestEntry(t, repo, "session-purge-dispatch", "task-purge-dispatch", "prompt", QueuedByUser, nil, nil)
	reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
	if err != nil || reserved == nil {
		t.Fatalf("reserve ordinary head = %#v, err=%v", reserved, err)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := PurgeTaskInTransaction(ctx, tx, repo.db, source.TaskID, []string{source.SessionID})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatalf("removed visible rows = %d, want 0", removed)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("dispatch claims after task purge = %#v, want empty", pending)
	}
}

func TestSQLiteTaskPurgePreservesAttachmentCleanup(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	cleanup := AttachmentCleanup{
		SessionID: "session-purge-cleanup", EntryID: "entry-purge-cleanup",
		OperationID: "operation-purge-cleanup", TaskID: "task-purge-cleanup",
		Attachments: []MessageAttachment{{AttachmentID: "attachment-purge-cleanup"}},
	}
	if err := repo.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
		t.Fatal(err)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PurgeTaskInTransaction(ctx, tx, repo.db, cleanup.TaskID, []string{cleanup.SessionID}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].OperationID != cleanup.OperationID {
		t.Fatalf("attachment cleanups after task purge = %#v, want retained obligation", pending)
	}
}

func TestSQLiteTaskPurgePreservesOtherTaskRecoveryInSharedSession(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	const (
		sessionID = "session-shared-purge"
		taskA     = "task-shared-purge-a"
		taskB     = "task-shared-purge-b"
	)
	entryA := insertTestEntry(t, repo, sessionID, taskA, "prompt-a", QueuedByUser, nil, nil)
	entryB := insertTestEntry(t, repo, sessionID, taskB, "prompt-b", QueuedByUser, nil, nil)
	for _, wantID := range []string{entryA.ID, entryB.ID} {
		reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, sessionID)
		if err != nil || reserved == nil || reserved.ID != wantID {
			t.Fatalf("reserve %s = %#v, err=%v", wantID, reserved, err)
		}
	}
	for _, cleanup := range []AttachmentCleanup{
		{
			SessionID: sessionID, EntryID: entryA.ID, OperationID: "cleanup-a", TaskID: taskA,
			Attachments: []MessageAttachment{{AttachmentID: "attachment-a"}},
		},
		{
			SessionID: sessionID, EntryID: entryB.ID, OperationID: "cleanup-b", TaskID: taskB,
			Attachments: []MessageAttachment{{AttachmentID: "attachment-b"}},
		},
	} {
		if err := repo.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
			t.Fatal(err)
		}
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PurgeTaskInTransaction(ctx, tx, repo.db, taskA, []string{sessionID}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	claims, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].Message.ID != entryB.ID {
		t.Fatalf("remaining dispatch claims = %#v, want only %s", claims, entryB.ID)
	}
	cleanups, err := repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanups) != 2 {
		t.Fatalf("remaining attachment cleanups = %#v, want both durable obligations", cleanups)
	}
}

func TestSQLiteTaskPurgeCreatesAbsentRecoveryTables(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PurgeTaskInTransaction(
		ctx, tx, repo.db, "task-without-recovery-tables", []string{"session-without-recovery-tables"},
	); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var claims int
	if err := repo.db.GetContext(ctx, &claims, `SELECT COUNT(*) FROM queue_dispatch_claims`); err != nil {
		t.Fatalf("query recovery table after purge: %v", err)
	}
}

func TestSQLiteRepositoryTaskPurgeDiscoversOrdinaryDispatchSession(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := insertTestEntry(t, repo, "session-direct-purge", "task-direct-purge", "prompt", QueuedByUser, nil, nil)
	reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
	if err != nil || reserved == nil {
		t.Fatalf("reserve ordinary head = %#v, err=%v", reserved, err)
	}
	if _, err := repo.PurgeTask(ctx, source.TaskID); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("dispatch claims after direct task purge = %#v, want empty", pending)
	}
}

func TestSQLiteAttachmentCleanupMigratesAndTransfersCurrentSession(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	_, err := repo.db.ExecContext(ctx, `
		CREATE TABLE queue_attachment_cleanups (
			session_id TEXT NOT NULL,
			entry_id TEXT NOT NULL,
			operation_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL,
			owner_id TEXT NOT NULL DEFAULT '',
			lease_id TEXT NOT NULL DEFAULT '',
			remove_entry INTEGER NOT NULL DEFAULT 0,
			claim_pending INTEGER NOT NULL DEFAULT 0,
			entry_fingerprint TEXT NOT NULL DEFAULT '',
			attachments_json TEXT NOT NULL DEFAULT '[]',
			created_at TIMESTAMP NOT NULL,
			PRIMARY KEY (session_id, entry_id, operation_id)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	const (
		sourceSessionID      = "legacy-cleanup-source"
		destinationSessionID = "legacy-cleanup-destination"
	)
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO queue_attachment_cleanups
			(session_id, entry_id, operation_id, task_id, attachments_json, created_at)
		VALUES (?, 'legacy-entry', 'legacy-operation', 'legacy-task', '[]', CURRENT_TIMESTAMP)
	`, sourceSessionID); err != nil {
		t.Fatal(err)
	}

	cleanups, err := repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanups) != 1 || cleanups[0].CurrentSessionID != sourceSessionID {
		t.Fatalf("migrated cleanup = %#v", cleanups)
	}
	if err := repo.TransferSession(ctx, sourceSessionID, destinationSessionID); err != nil {
		t.Fatal(err)
	}
	cleanups, err = repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanups) != 1 ||
		cleanups[0].SessionID != sourceSessionID ||
		cleanups[0].CurrentSessionID != destinationSessionID {
		t.Fatalf("transferred cleanup = %#v", cleanups)
	}
}

func TestSQLiteStaleAttachmentCleanupUpsertPreservesTransferredSession(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	cleanup := AttachmentCleanup{
		SessionID: "cleanup-stale-upsert-old", EntryID: "cleanup-stale-upsert-entry",
		OperationID: "cleanup-stale-upsert-operation", TaskID: "cleanup-stale-upsert-task",
		CurrentSessionID: "cleanup-stale-upsert-old",
		Attachments:      []MessageAttachment{{AttachmentID: "cleanup-stale-upsert-attachment"}},
	}
	if err := repo.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	const destinationSessionID = "cleanup-stale-upsert-new"
	if err := repo.TransferSession(ctx, cleanup.SessionID, destinationSessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.CurrentSessionID != destinationSessionID {
		t.Fatalf("cleanup after stale upsert = %#v", stored)
	}
}

func TestSQLiteAttachmentCleanupUpsertReadsTransferredQueueEntry(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	entry := insertTestEntry(t, repo, "cleanup-entry-old", "cleanup-entry-task", "prompt", QueuedByUser, nil, nil)
	const destinationSessionID = "cleanup-entry-new"
	if err := repo.TransferSession(ctx, entry.SessionID, destinationSessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAttachmentCleanup(ctx, AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "cleanup-entry-operation",
		TaskID: entry.TaskID, CurrentSessionID: entry.SessionID,
		Attachments: []MessageAttachment{{AttachmentID: "cleanup-entry-attachment"}},
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetAttachmentCleanup(ctx, entry.SessionID, entry.ID, "cleanup-entry-operation")
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.CurrentSessionID != destinationSessionID {
		t.Fatalf("cleanup after transferred queue entry = %#v", stored)
	}
}
