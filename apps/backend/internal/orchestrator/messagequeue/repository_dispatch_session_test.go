package messagequeue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestSQLiteTransferSessionMovesOrdinaryDispatchClaim(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := insertTestEntry(t, repo, "session-transfer-old", "task-transfer", "prompt", QueuedByUser, nil, nil)
	reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
	if err != nil || reserved == nil {
		t.Fatalf("reserve ordinary head = %#v, err=%v", reserved, err)
	}
	if err := repo.TransferSession(ctx, source.SessionID, "session-transfer-new"); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Message.SessionID != "session-transfer-new" {
		t.Fatalf("dispatch claims after transfer = %#v", pending)
	}
	if err := repo.MarkPendingQueueDispatchAccepted(ctx, reserved); !errors.Is(err, ErrQueueDispatchClaimChanged) {
		t.Fatalf("stale migrated claim acceptance = %v, want %v", err, ErrQueueDispatchClaimChanged)
	}
	if err := repo.DeletePendingQueueDispatch(ctx, reserved); !errors.Is(err, ErrQueueDispatchClaimChanged) {
		t.Fatalf("stale migrated claim acknowledgement = %v, want %v", err, ErrQueueDispatchClaimChanged)
	}
	pending, err = repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Message.dispatchAttemptID == reserved.dispatchAttemptID {
		t.Fatalf("transferred claim attempt was not rotated: reserved=%q pending=%#v", reserved.dispatchAttemptID, pending)
	}
	if err := repo.MarkPendingQueueDispatchAccepted(ctx, &pending[0].Message); err != nil {
		t.Fatalf("mark current migrated claim: %v", err)
	}
}

func TestSQLiteReplaceSessionInvalidatesOrdinaryDispatchClaims(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := insertTestEntry(t, repo, "session-replace", "task-replace", "prompt", QueuedByUser, nil, nil)
	reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
	if err != nil || reserved == nil {
		t.Fatalf("reserve ordinary head = %#v, err=%v", reserved, err)
	}
	if err := repo.ReplaceSession(ctx, source.SessionID, nil, nil); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("dispatch claims after replacement = %#v, want empty", pending)
	}
}

func TestSQLiteStaleOrdinarySettlementCannotResurrectAfterSessionMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *Service, *QueuedMessage) error
		settle func(context.Context, *Service, *QueuedMessage) error
	}{
		{
			name: "transfer before restore",
			mutate: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				return service.TransferSession(ctx, msg.SessionID, "session-transfer-destination")
			},
			settle: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				_, err := service.RestoreMessage(ctx, msg)
				return err
			},
		},
		{
			name: "replace before requeue",
			mutate: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				return service.repo.ReplaceSession(ctx, msg.SessionID, nil, nil)
			},
			settle: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				return service.RequeueAtHead(ctx, msg)
			},
		},
		{
			name: "clear before restore",
			mutate: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				_, err := service.CancelAll(ctx, msg.SessionID)
				return err
			},
			settle: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				_, err := service.RestoreMessage(ctx, msg)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newTestSQLiteRepo(t)
			service := setupService(t)
			service.repo = repo
			source, err := service.QueueMessage(
				ctx, "session-stale", "task-stale", "prompt", "", QueuedByUser, false, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			reserved, ok := service.ReserveQueued(ctx, source.SessionID)
			if !ok || reserved.ID != source.ID {
				t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
			}
			if err := tc.mutate(ctx, service, reserved); err != nil {
				t.Fatal(err)
			}
			if err := tc.settle(ctx, service, reserved); !errors.Is(err, ErrQueueDispatchClaimChanged) {
				t.Fatalf("stale settlement error = %v, want %v", err, ErrQueueDispatchClaimChanged)
			}
			if entries, err := repo.ListBySession(ctx, source.SessionID); err != nil || len(entries) != 0 {
				t.Fatalf("stale source queue = %#v, err=%v", entries, err)
			}
		})
	}
}

func TestDestructiveTakeSettlementIsFencedAfterTransfer(t *testing.T) {
	factories := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}
	takes := []struct {
		name string
		take func(context.Context, Repository, *QueuedMessage) (*QueuedMessage, error)
	}{
		{
			name: "head",
			take: func(ctx context.Context, repo Repository, msg *QueuedMessage) (*QueuedMessage, error) {
				return repo.TakeHead(ctx, msg.SessionID)
			},
		},
		{
			name: "by id",
			take: func(ctx context.Context, repo Repository, msg *QueuedMessage) (*QueuedMessage, error) {
				return repo.TakeByID(ctx, msg.SessionID, msg.ID)
			},
		},
	}
	for _, factory := range factories {
		for _, take := range takes {
			t.Run(factory.name+"/"+take.name, func(t *testing.T) {
				ctx := context.Background()
				repo := factory.new(t)
				source := insertTestEntry(t, repo, "session-direct-old", "task-direct", "prompt", QueuedByUser, nil, nil)
				reserved, err := take.take(ctx, repo, source)
				if err != nil || reserved == nil {
					t.Fatalf("take = %#v, err=%v", reserved, err)
				}
				if _, _, captured := reserved.ReservationGenerations(); !captured {
					t.Fatal("destructive take did not capture settlement generation")
				}
				if err := repo.TransferSession(ctx, source.SessionID, "session-direct-new"); err != nil {
					t.Fatal(err)
				}
				if err := repo.Restore(ctx, reserved, 0); !errors.Is(err, ErrQueueDispatchClaimChanged) {
					t.Fatalf("stale direct-take restore = %v, want %v", err, ErrQueueDispatchClaimChanged)
				}
			})
		}
	}
}

func TestLifecycleAcknowledgementRequiresCurrentReservation(t *testing.T) {
	factories := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}
	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repo := factory.new(t)
			source := insertTestEntry(
				t, repo, "session-lifecycle-attempt", "task-lifecycle-attempt", "prompt",
				QueuedByWorkflow, nil, map[string]interface{}{MetadataLifecycleDurable: true},
			)
			first, err := repo.ReserveHead(ctx, source.SessionID)
			if err != nil || first == nil {
				t.Fatalf("first reserve = %#v, err=%v", first, err)
			}
			second, err := repo.ReserveHead(ctx, source.SessionID)
			if err != nil || second == nil {
				t.Fatalf("second reserve = %#v, err=%v", second, err)
			}
			if first.lifecycleReservationID == "" || first.lifecycleReservationID == second.lifecycleReservationID {
				t.Fatalf("lifecycle reservation ids = %q, %q", first.lifecycleReservationID, second.lifecycleReservationID)
			}
			if err := repo.AcknowledgeReserved(ctx, first); !errors.Is(err, ErrLifecycleReservationChanged) {
				t.Fatalf("stale lifecycle acknowledgement = %v, want %v", err, ErrLifecycleReservationChanged)
			}
			if err := repo.AcknowledgeReserved(ctx, second); err != nil {
				t.Fatalf("current lifecycle acknowledgement: %v", err)
			}
		})
	}
}

func TestSQLiteQueueDispatchClaimMigratesLegacyAttemptID(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	_, err := repo.db.ExecContext(ctx, `
		CREATE TABLE queue_dispatch_claims (
			entry_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			message_json TEXT NOT NULL,
			accepted INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []struct {
		msg      QueuedMessage
		accepted int
	}{
		{
			msg: QueuedMessage{
				ID: "legacy-dispatch-pending", SessionID: "legacy-session-pending", TaskID: "legacy-task",
				Content: "legacy pending prompt", QueuedBy: QueuedByUser,
			},
		},
		{
			msg: QueuedMessage{
				ID: "legacy-dispatch-accepted", SessionID: "legacy-session-accepted", TaskID: "legacy-task",
				Content: "legacy accepted prompt", QueuedBy: QueuedByUser,
			},
			accepted: 1,
		},
	} {
		messageJSON, err := json.Marshal(legacy.msg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.db.ExecContext(ctx, `
			INSERT INTO queue_dispatch_claims
				(entry_id, session_id, message_json, accepted, created_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, legacy.msg.ID, legacy.msg.SessionID, string(messageJSON), legacy.accepted); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("migrated dispatch count = %d, want 2", len(pending))
	}
	attemptIDs := make(map[string]struct{}, len(pending))
	for i := range pending {
		attemptID := pending[i].Message.dispatchAttemptID
		if attemptID == "" {
			t.Fatalf("migrated dispatch = %#v, want non-empty attempt id", pending[i])
		}
		if _, exists := attemptIDs[attemptID]; exists {
			t.Fatalf("duplicate migrated dispatch attempt id %q", attemptID)
		}
		attemptIDs[attemptID] = struct{}{}
		if pending[i].Accepted {
			err = repo.DeletePendingQueueDispatch(ctx, &pending[i].Message)
		} else {
			err = repo.Restore(ctx, &pending[i].Message, 0)
		}
		if err != nil {
			t.Fatalf("settle migrated dispatch %q: %v", pending[i].Message.ID, err)
		}
	}
	pending, err = repo.ListPendingQueueDispatches(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after migrated settlement = %#v, err=%v", pending, err)
	}
}
