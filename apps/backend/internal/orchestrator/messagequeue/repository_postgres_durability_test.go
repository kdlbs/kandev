package messagequeue

import (
	"context"
	"errors"
	"testing"
)

func TestPostgresRepository_DurableQueueRecoveryTables(t *testing.T) {
	ctx := context.Background()
	repo := newTestPostgresRepo(t)
	persistent := repo.(*sqliteRepository)

	t.Run("attachment cleanup replays schema and upsert", func(t *testing.T) {
		cleanup := AttachmentCleanup{
			SessionID: "cleanup-session", EntryID: "cleanup-entry", OperationID: "cleanup-op",
			TaskID: "cleanup-task", OwnerID: "owner-1", LeaseID: "lease-1",
			Attachments: []MessageAttachment{{Name: "first.txt", Data: "first"}},
		}
		if err := persistent.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
			t.Fatal(err)
		}
		cleanup.OwnerID = "owner-2"
		cleanup.Attachments = []MessageAttachment{{Name: "second.txt", Data: "second"}}
		if err := persistent.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			cleanups, err := persistent.ListAttachmentCleanups(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(cleanups) != 1 || cleanups[0].OwnerID != "owner-2" || cleanups[0].Attachments[0].Name != "second.txt" {
				t.Fatalf("attachment cleanups = %#v", cleanups)
			}
		}
		if err := persistent.DeleteAttachmentCleanup(ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("session transfer compensation replays schema and upsert", func(t *testing.T) {
		compensation := SessionTransferCompensation{
			OperationID: "transfer-operation",
			TaskID:      "transfer-task", FromSessionID: "transfer-old", ToSessionID: "transfer-new",
			EntryIDs: []string{"entry-1"},
		}
		if err := persistent.UpsertSessionTransferCompensation(ctx, compensation); err != nil {
			t.Fatal(err)
		}
		compensation.EntryIDs = []string{"entry-1", "entry-2"}
		if err := persistent.UpsertSessionTransferCompensation(ctx, compensation); err != nil {
			t.Fatal(err)
		}
		compensations, err := persistent.ListSessionTransferCompensations(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(compensations) != 1 || len(compensations[0].EntryIDs) != 2 || compensations[0].EntryIDs[1] != "entry-2" {
			t.Fatalf("session transfer compensations = %#v", compensations)
		}
		if err := persistent.DeleteSessionTransferCompensation(ctx, compensation.OperationID, compensation.TaskID, compensation.FromSessionID, compensation.ToSessionID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("send now claim records acceptance", func(t *testing.T) {
		source := insertTestEntry(t, repo, "send-now-session", "send-now-task", "prompt", QueuedByUser, nil, nil)
		claim, err := repo.ClaimSendNow(ctx, source.SessionID, []QueuedMessage{*source})
		if err != nil {
			t.Fatal(err)
		}
		if err := persistent.MarkPendingSendNowClaimAccepted(ctx, claim); err != nil {
			t.Fatal(err)
		}
		claims, err := persistent.ListPendingSendNowClaims(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(claims) != 1 || !claims[0].Accepted || claims[0].Claim.Dispatch.ID != claim.Dispatch.ID {
			t.Fatalf("pending Send Now claims = %#v", claims)
		}
		if err := repo.AcknowledgeSendNowClaim(ctx, claim); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ordinary dispatch claim records acceptance", func(t *testing.T) {
		source := insertTestEntry(t, repo, "dispatch-session", "dispatch-task", "prompt", QueuedByUser, nil, nil)
		reserved, enabled, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		if !enabled || reserved == nil || reserved.ID != source.ID {
			t.Fatalf("ordinary reservation = %#v, enabled=%t", reserved, enabled)
		}
		if err := persistent.MarkPendingQueueDispatchAccepted(ctx, reserved); err != nil {
			t.Fatal(err)
		}
		dispatches, err := persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 1 || !dispatches[0].Accepted || dispatches[0].Message.ID != source.ID {
			t.Fatalf("pending ordinary dispatches = %#v", dispatches)
		}
		if err := persistent.DeletePendingQueueDispatch(ctx, reserved); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("task purge preserves attachment recovery obligation", func(t *testing.T) {
		source := insertTestEntry(t, repo, "purge-session", "purge-task", "prompt", QueuedByUser, nil, nil)
		if err := persistent.UpsertAttachmentCleanup(ctx, AttachmentCleanup{
			SessionID: source.SessionID, EntryID: source.ID, OperationID: "purge-operation",
			TaskID: source.TaskID, Attachments: []MessageAttachment{{AttachmentID: "purge-attachment"}},
		}); err != nil {
			t.Fatal(err)
		}
		if reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID); err != nil || reserved == nil {
			t.Fatalf("reserve purge source = %#v, err=%v", reserved, err)
		}
		if _, err := repo.PurgeTask(ctx, source.TaskID); err != nil {
			t.Fatal(err)
		}
		dispatches, err := persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		cleanups, err := persistent.ListAttachmentCleanups(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 0 || len(cleanups) != 1 {
			t.Fatalf("recovery rows after purge: dispatches=%#v cleanups=%#v", dispatches, cleanups)
		}
	})

	t.Run("session mutation reconciles ordinary dispatch claims", func(t *testing.T) {
		source := insertTestEntry(t, repo, "mutation-old", "mutation-task", "prompt", QueuedByUser, nil, nil)
		if reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID); err != nil || reserved == nil {
			t.Fatalf("reserve mutation source = %#v, err=%v", reserved, err)
		}
		if err := repo.TransferSession(ctx, source.SessionID, "mutation-new"); err != nil {
			t.Fatal(err)
		}
		dispatches, err := persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 1 || dispatches[0].Message.SessionID != "mutation-new" {
			t.Fatalf("transferred dispatch claims = %#v", dispatches)
		}
		if err := repo.ReplaceSession(ctx, "mutation-new", nil, nil); err != nil {
			t.Fatal(err)
		}
		dispatches, err = persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 0 {
			t.Fatalf("dispatch claims after replacement = %#v", dispatches)
		}
	})
}

func TestPostgresAttachmentCleanupMigratesAndTransfersCurrentSession(t *testing.T) {
	ctx := context.Background()
	repo := newTestPostgresRepo(t).(*sqliteRepository)
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE IF EXISTS queue_attachment_cleanups`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `
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
	`); err != nil {
		t.Fatal(err)
	}
	const (
		sourceSessionID = "postgres-cleanup-source"
		middleSessionID = "postgres-cleanup-middle"
		finalSessionID  = "postgres-cleanup-final"
	)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO queue_attachment_cleanups
			(session_id, entry_id, operation_id, task_id, attachments_json, created_at)
		VALUES (?, 'postgres-cleanup-entry', 'postgres-cleanup-operation',
			'postgres-cleanup-task', '[]', CURRENT_TIMESTAMP)
	`), sourceSessionID); err != nil {
		t.Fatal(err)
	}

	cleanups, err := repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanups) != 1 || cleanups[0].CurrentSessionID != sourceSessionID {
		t.Fatalf("migrated PostgreSQL cleanup = %#v", cleanups)
	}
	if err := repo.TransferSession(ctx, sourceSessionID, middleSessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.TransferSession(ctx, middleSessionID, finalSessionID); err != nil {
		t.Fatal(err)
	}
	cleanups, err = repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanups) != 1 ||
		cleanups[0].SessionID != sourceSessionID ||
		cleanups[0].CurrentSessionID != finalSessionID {
		t.Fatalf("multi-hop PostgreSQL cleanup = %#v", cleanups)
	}
}

type replaceBeforeReservationRepository struct {
	Repository
	replacement Repository
}

func (r *replaceBeforeReservationRepository) ReserveHeadIfAutoRun(
	ctx context.Context,
	sessionID string,
) (*QueuedMessage, bool, error) {
	entries, err := r.replacement.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, true, err
	}
	if err := r.replacement.ReplaceSession(ctx, sessionID, entries, nil); err != nil {
		return nil, true, err
	}
	return r.Repository.ReserveHeadIfAutoRun(ctx, sessionID)
}

func TestPostgresRepository_ReservationCapturesGenerationAfterConcurrentReplacement(t *testing.T) {
	repoA, repoB, _ := newTestPostgresRepoPair(t)
	ctx := context.Background()
	source := insertTestEntry(t, repoA, "reservation-race-session", "reservation-race-task", "prompt", QueuedByUser, nil, nil)
	service := setupService(t)
	service.repo = &replaceBeforeReservationRepository{Repository: repoA, replacement: repoB}

	reserved, exists, autoRun := service.ReserveQueuedWithAutoRun(ctx, source.SessionID)
	if !exists || !autoRun || reserved == nil {
		t.Fatalf("reservation = %#v, exists=%t, autoRun=%t", reserved, exists, autoRun)
	}
	generation, err := repoA.SessionGeneration(ctx, source.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reserved.reservationGenerationsCaptured || reserved.reservationSessionGeneration != generation {
		t.Fatalf(
			"reservation generation = %d (captured=%t), want current %d",
			reserved.reservationSessionGeneration, reserved.reservationGenerationsCaptured, generation,
		)
	}
}

func TestPostgresRepository_TransferFenceOwnershipIsOperationScoped(t *testing.T) {
	repoA, repoB, _ := newTestPostgresRepoPair(t)
	ctx := context.Background()
	persistenceA := repoA.(sessionTransferCompensationRepository)
	persistenceB := repoB.(sessionTransferCompensationRepository)
	first := SessionTransferCompensation{
		OperationID: "transfer-first",
		TaskID:      "transfer-task", FromSessionID: "transfer-old", ToSessionID: "transfer-new",
		EntryIDs: []string{"entry-first"},
	}
	if err := persistenceA.UpsertSessionTransferCompensation(ctx, first); err != nil {
		t.Fatal(err)
	}
	replacement := first
	replacement.OperationID = "transfer-replacement"
	replacement.EntryIDs = []string{"entry-replacement"}
	if err := persistenceB.UpsertSessionTransferCompensation(ctx, replacement); !errors.Is(err, ErrSessionTransferInProgress) {
		t.Fatalf("replacement error = %v, want %v", err, ErrSessionTransferInProgress)
	}
	if err := persistenceB.DeleteSessionTransferCompensation(
		ctx, replacement.OperationID, first.TaskID, first.FromSessionID, first.ToSessionID,
	); !errors.Is(err, ErrSessionTransferOwnershipLost) {
		t.Fatalf("stale delete error = %v, want %v", err, ErrSessionTransferOwnershipLost)
	}
	stored, err := persistenceA.ListSessionTransferCompensations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].OperationID != first.OperationID {
		t.Fatalf("session transfer compensations = %#v", stored)
	}
}

func TestPostgresRepository_ActiveSessionTransferFencesMutationsAcrossInstances(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, Repository, *QueuedMessage, *QueuedMessage) error
	}{
		{
			name: "edit",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				return repo.UpdateContent(ctx, first.SessionID, first.ID, "edited", nil, QueuedByUser)
			},
		},
		{
			name: "purge",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.PurgeSession(ctx, first.SessionID)
				return err
			},
		},
		{
			name: "purge task",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.PurgeTask(ctx, first.TaskID)
				return err
			},
		},
		{
			name: "merge",
			mutate: func(ctx context.Context, repo Repository, _, second *QueuedMessage) error {
				_, err := repo.MergeIntoAbove(ctx, second.SessionID, second.ID, QueuedByUser)
				return err
			},
		},
		{
			name: "reorder",
			mutate: func(ctx context.Context, repo Repository, first, second *QueuedMessage) error {
				return repo.ReorderEntries(ctx, first.SessionID, []string{second.ID, first.ID})
			},
		},
		{
			name: "take pending move",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.TakePendingMove(ctx, first.SessionID)
				return err
			},
		},
		{
			name: "transfer",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				return repo.TransferSession(ctx, first.SessionID, "session-third")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoA, repoB, _ := newTestPostgresRepoPair(t)
			ctx := context.Background()
			first := insertTestEntry(t, repoA, "session-old", "task", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repoA, "session-old", "task", "second", QueuedByUser, nil, nil)
			if err := repoA.SetPendingMove(ctx, first.SessionID, &PendingMove{
				MoveID: "move", TaskID: "task", WorkflowID: "workflow", WorkflowStepID: "step",
			}); err != nil {
				t.Fatal(err)
			}
			persistence := repoA.(sessionTransferCompensationRepository)
			if err := persistence.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
				OperationID: "transfer-operation",
				TaskID:      "task", FromSessionID: first.SessionID, ToSessionID: "session-new",
			}); err != nil {
				t.Fatal(err)
			}

			if err := test.mutate(ctx, repoB, first, second); !errors.Is(err, ErrSessionTransferInProgress) {
				t.Fatalf("mutation error = %v, want %v", err, ErrSessionTransferInProgress)
			}
		})
	}
}

func TestPostgresRepository_ActiveSessionTransferFencesDispatchSettlementAcrossInstances(t *testing.T) {
	tests := []struct {
		name   string
		settle func(context.Context, *sqliteRepository, *QueuedMessage) error
	}{
		{name: "mark accepted", settle: func(ctx context.Context, repo *sqliteRepository, msg *QueuedMessage) error {
			return repo.MarkPendingQueueDispatchAccepted(ctx, msg)
		}},
		{name: "delete", settle: func(ctx context.Context, repo *sqliteRepository, msg *QueuedMessage) error {
			return repo.DeletePendingQueueDispatch(ctx, msg)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoA, repoB, _ := newTestPostgresRepoPair(t)
			ctx := context.Background()
			entry := insertTestEntry(t, repoA, "session-old", "task", "first", QueuedByUser, nil, nil)
			reserved, err := repoA.ReserveHead(ctx, entry.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			persistentA := repoA.(sessionTransferCompensationRepository)
			if err := persistentA.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
				OperationID: "transfer-operation",
				TaskID:      entry.TaskID, FromSessionID: entry.SessionID, ToSessionID: "session-new",
			}); err != nil {
				t.Fatal(err)
			}

			err = test.settle(ctx, repoB.(*sqliteRepository), reserved)
			if !errors.Is(err, ErrSessionTransferInProgress) {
				t.Fatalf("settlement error = %v, want %v", err, ErrSessionTransferInProgress)
			}
		})
	}
}

func TestPostgresRepository_ActiveSessionTransferFencesSendNowSettlementAcrossInstances(t *testing.T) {
	tests := []struct {
		name   string
		settle func(context.Context, *sqliteRepository, *SendNowClaim) error
	}{
		{name: "mark accepted", settle: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
			return repo.MarkPendingSendNowClaimAccepted(ctx, claim)
		}},
		{name: "delete", settle: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
			return repo.DeletePendingSendNowClaim(ctx, claim)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoA, repoB, _ := newTestPostgresRepoPair(t)
			ctx := context.Background()
			entry := insertTestEntry(t, repoA, "session-old", "task", "first", QueuedByUser, nil, nil)
			claim, err := repoA.ClaimSendNow(ctx, entry.SessionID, []QueuedMessage{*entry})
			if err != nil {
				t.Fatal(err)
			}
			persistentA := repoA.(sessionTransferCompensationRepository)
			if err := persistentA.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
				OperationID: "transfer-operation",
				TaskID:      entry.TaskID, FromSessionID: entry.SessionID, ToSessionID: "session-new",
			}); err != nil {
				t.Fatal(err)
			}

			err = test.settle(ctx, repoB.(*sqliteRepository), claim)
			if !errors.Is(err, ErrSessionTransferInProgress) {
				t.Fatalf("settlement error = %v, want %v", err, ErrSessionTransferInProgress)
			}
		})
	}
}

func TestPostgresRepository_ActiveSessionTransferFencesEditLeaseAcrossInstances(t *testing.T) {
	repoA, repoB, _ := newTestPostgresRepoPair(t)
	ctx := context.Background()
	entry := insertTestEntry(t, repoA, "session-old", "task", "first", QueuedByUser, nil, nil)
	persistentA := repoA.(sessionTransferCompensationRepository)
	if err := persistentA.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
		OperationID: "transfer-operation",
		TaskID:      entry.TaskID, FromSessionID: entry.SessionID, ToSessionID: "session-new",
	}); err != nil {
		t.Fatal(err)
	}
	serviceB := newAutoMergeTestServiceWithRepository(t, repoB, DefaultMaxPerSession)

	lease, err := serviceB.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")

	if !errors.Is(err, ErrSessionTransferInProgress) {
		t.Fatalf("BeginEdit error = %v, want %v", err, ErrSessionTransferInProgress)
	}
	if lease != nil {
		t.Fatalf("lease = %#v, want nil", lease)
	}
}
