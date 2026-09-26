package messagequeue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestSQLiteSendNowClaimPersistsOrdinarySourceRecovery(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	source := insertTestEntry(t, repo, "session-1", "task-1", "ordinary prompt", QueuedByUser, nil, nil)

	claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*source})
	if err != nil {
		t.Fatal(err)
	}
	if entries, listErr := repo.ListBySession(ctx, "session-1"); listErr != nil || len(entries) != 0 {
		t.Fatalf("queue after claim = %#v, err=%v, want ordinary source removed", entries, listErr)
	}
	persistent, ok := repo.(interface {
		ListPendingSendNowClaims(context.Context) ([]PendingSendNowClaim, error)
	})
	if !ok {
		t.Fatal("SQLite queue repository does not persist pending Send Now claims")
	}
	pending, err := persistent.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Claim.Dispatch.ID != claim.Dispatch.ID {
		t.Fatalf("pending claims = %#v, want dispatch %s", pending, claim.Dispatch.ID)
	}

	if err := repo.RestoreSendNowClaim(ctx, &pending[0].Claim); err != nil {
		t.Fatal(err)
	}
	pending, err = persistent.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending claims after restore = %#v", pending)
	}
	entries, err := repo.ListBySession(ctx, "session-1")

	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != source.ID {
		t.Fatalf("restored queue = %#v, want source %s", entries, source.ID)
	}
}
func TestSQLiteTransferSessionMovesPendingSendNowClaim(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprintf("accepted=%t", accepted), func(t *testing.T) {
			ctx := context.Background()
			repo := newTestSQLiteRepo(t).(*sqliteRepository)
			source := insertTestEntry(
				t, repo, "session-old", "task-1", "durable source", QueuedByWorkflow, nil,
				map[string]interface{}{MetadataLifecycleDurable: true},
			)
			claim, err := repo.ClaimSendNow(ctx, source.SessionID, []QueuedMessage{*source})
			if err != nil {
				t.Fatal(err)
			}
			if accepted {
				if err := repo.MarkPendingSendNowClaimAccepted(ctx, claim); err != nil {
					t.Fatal(err)
				}
			}

			if err := repo.TransferSession(ctx, "session-old", "session-new"); err != nil {
				t.Fatal(err)
			}

			pending, err := repo.ListPendingSendNowClaims(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 1 {
				t.Fatalf("pending claims = %#v, want one transferred claim", pending)
			}
			transferred := pending[0]
			if transferred.Claim.Sources[0].SessionID != "session-new" ||
				transferred.Claim.Dispatch.SessionID != "session-new" {
				t.Fatalf("transferred claim = %#v, want session-new", transferred.Claim)
			}
			if transferred.Accepted != accepted {
				t.Fatalf("transferred accepted = %t, want %t", transferred.Accepted, accepted)
			}
			if accepted {
				err = repo.AcknowledgeSendNowClaim(ctx, &transferred.Claim)
			} else {
				err = repo.RestoreSendNowClaim(ctx, &transferred.Claim)
			}
			if err != nil {
				t.Fatal(err)
			}
			entries, err := repo.ListBySession(ctx, "session-new")
			if err != nil {
				t.Fatal(err)
			}
			if accepted && len(entries) != 0 {
				t.Fatalf("acknowledged destination queue = %#v, want empty", entries)
			}
			if !accepted && (len(entries) != 1 || entries[0].ID != source.ID || entries[0].IsReservedInFlight()) {
				t.Fatalf("restored destination queue = %#v", entries)
			}
		})
	}
}

func TestSQLiteTransferSessionIdentitiesRebindsPendingSendNowClaim(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprintf("accepted=%t", accepted), func(t *testing.T) {
			ctx := context.Background()
			repo := newTestSQLiteRepo(t).(*sqliteRepository)
			sourceIdentity := QueueSessionIdentity{
				TaskID:               "task-1",
				SessionID:            "session-old",
				SessionIncarnationID: "incarnation-old",
			}
			destinationIdentity := QueueSessionIdentity{
				TaskID:               "task-1",
				SessionID:            "session-new",
				SessionIncarnationID: "incarnation-new",
			}
			seedQueueSessionIdentity(t, repo, sourceIdentity)
			seedQueueSessionIdentity(t, repo, destinationIdentity)
			destinationPrime := &QueuedMessage{
				ID: "destination-prime", TaskID: destinationIdentity.TaskID, SessionID: destinationIdentity.SessionID,
				Content: "already settled", QueuedBy: QueuedByUser,
			}
			if err := repo.InsertForSession(ctx, destinationIdentity, destinationPrime, DefaultMaxPerSession); err != nil {
				t.Fatal(err)
			}
			primeClaim, err := repo.ClaimSendNowForSession(ctx, destinationIdentity, []QueuedMessage{*destinationPrime})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.AcknowledgeSendNowClaim(ctx, primeClaim); err != nil {
				t.Fatal(err)
			}
			source := &QueuedMessage{
				ID: "source", TaskID: sourceIdentity.TaskID, SessionID: sourceIdentity.SessionID,
				Content: "durable source", QueuedBy: QueuedByWorkflow,
				Metadata: map[string]interface{}{MetadataLifecycleDurable: true},
			}
			if err := repo.InsertForSession(ctx, sourceIdentity, source, DefaultMaxPerSession); err != nil {
				t.Fatal(err)
			}
			claim, err := repo.ClaimSendNowForSession(ctx, sourceIdentity, []QueuedMessage{*source})
			if err != nil {
				t.Fatal(err)
			}
			sourceGeneration := claim.OperationGeneration
			if accepted {
				if err := repo.MarkPendingSendNowClaimAccepted(ctx, claim); err != nil {
					t.Fatal(err)
				}
			}

			if err := repo.TransferSessionIdentities(ctx, sourceIdentity, destinationIdentity); err != nil {
				t.Fatal(err)
			}

			pending, err := repo.ListPendingSendNowClaims(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 1 {
				t.Fatalf("pending claims = %#v, want one transferred claim", pending)
			}
			transferred := pending[0]
			if transferred.Claim.Identity != destinationIdentity {
				t.Fatalf("transferred claim identity = %#v, want %#v", transferred.Claim.Identity, destinationIdentity)
			}
			if transferred.Claim.OperationGeneration != transferred.Claim.SessionGeneration {
				t.Fatalf("transferred claim generations = operation %d, session %d", transferred.Claim.OperationGeneration, transferred.Claim.SessionGeneration)
			}
			if transferred.Claim.OperationGeneration <= sourceGeneration {
				t.Fatalf("transferred operation generation = %d, want greater than source %d", transferred.Claim.OperationGeneration, sourceGeneration)
			}
			if accepted {
				err = repo.AcknowledgeSendNowClaim(ctx, &transferred.Claim)
			} else {
				err = repo.RestoreSendNowClaim(ctx, &transferred.Claim)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTransferredSendNowClaimFencesOriginalSourceActions(t *testing.T) {
	transfers := []struct {
		name string
		run  func(context.Context, *sqliteRepository, QueueSessionIdentity, QueueSessionIdentity) error
	}{
		{
			name: "direct",
			run: func(ctx context.Context, repo *sqliteRepository, source, destination QueueSessionIdentity) error {
				return repo.TransferSessionIdentities(ctx, source, destination)
			},
		},
		{
			name: "durable",
			run: func(ctx context.Context, repo *sqliteRepository, source, destination QueueSessionIdentity) error {
				service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
				return service.TransferSessionWithDurableAttachmentPreparation(
					ctx, source.TaskID, source.SessionID, destination.SessionID, nil, nil,
				)
			},
		},
	}
	actions := []struct {
		name string
		run  func(context.Context, *sqliteRepository, *SendNowClaim) error
	}{
		{name: "restore", run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
			return repo.RestoreSendNowClaim(ctx, claim)
		}},
		{name: "acknowledge", run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
			return repo.AcknowledgeSendNowClaim(ctx, claim)
		}},
		{name: "accept", run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
			return repo.MarkPendingSendNowClaimAccepted(ctx, claim)
		}},
		{name: "startup discard", run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
			return repo.DeletePendingSendNowClaim(ctx, claim)
		}},
	}

	for _, transfer := range transfers {
		for _, accepted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/accepted=%t", transfer.name, accepted), func(t *testing.T) {
				ctx := context.Background()
				repo := newTestSQLiteRepo(t).(*sqliteRepository)
				sourceIdentity := QueueSessionIdentity{
					TaskID:               "task-1",
					SessionID:            "session-old",
					SessionIncarnationID: "incarnation-old",
				}
				destinationIdentity := QueueSessionIdentity{
					TaskID:               sourceIdentity.TaskID,
					SessionID:            "session-new",
					SessionIncarnationID: "incarnation-new",
				}
				seedQueueSessionIdentity(t, repo, sourceIdentity)
				seedQueueSessionIdentity(t, repo, destinationIdentity)
				for _, source := range []*QueuedMessage{
					{ID: "first", TaskID: sourceIdentity.TaskID, SessionID: sourceIdentity.SessionID, Content: "first", QueuedBy: QueuedByUser},
					{ID: "second", TaskID: sourceIdentity.TaskID, SessionID: sourceIdentity.SessionID, Content: "second", QueuedBy: QueuedByUser},
				} {
					if err := repo.InsertForSession(ctx, sourceIdentity, source, DefaultMaxPerSession); err != nil {
						t.Fatal(err)
					}
				}
				sources, err := repo.ListBySession(ctx, sourceIdentity.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				claim, err := repo.ClaimSendNowForSession(ctx, sourceIdentity, sources)
				if err != nil {
					t.Fatal(err)
				}
				if accepted {
					if err := repo.MarkPendingSendNowClaimAccepted(ctx, claim); err != nil {
						t.Fatal(err)
					}
				}
				if err := transfer.run(ctx, repo, sourceIdentity, destinationIdentity); err != nil {
					t.Fatal(err)
				}

				for _, action := range actions {
					t.Run(action.name, func(t *testing.T) {
						err := action.run(ctx, repo, claim)
						if !errors.Is(err, ErrSendNowClaimChanged) && !errors.Is(err, ErrSessionIdentityMismatch) {
							t.Fatalf("old source %s = %v, want claim fencing error", action.name, err)
						}
						pending, err := repo.ListPendingSendNowClaims(ctx)
						if err != nil {
							t.Fatal(err)
						}
						if len(pending) != 1 {
							t.Fatalf("pending claims after stale %s = %#v, want successor claim", action.name, pending)
						}
						transferred := pending[0]
						if transferred.Claim.ClaimID != claim.ClaimID || transferred.Claim.Identity != destinationIdentity || transferred.Accepted != accepted {
							t.Fatalf("transferred claim after stale %s = %#v", action.name, transferred)
						}
						if len(transferred.Claim.Sources) != 2 ||
							transferred.Claim.Sources[0].Content != "first" ||
							transferred.Claim.Sources[1].Content != "second" ||
							transferred.Claim.Sources[0].SessionID != destinationIdentity.SessionID ||
							transferred.Claim.Sources[1].SessionID != destinationIdentity.SessionID {
							t.Fatalf("transferred FIFO sources after stale %s = %#v", action.name, transferred.Claim.Sources)
						}
					})
				}
			})
		}
	}
}

func TestSQLitePurgeRemovesDurableSendNowClaims(t *testing.T) {
	for _, purge := range []struct {
		name string
		run  func(context.Context, Repository) error
	}{
		{
			name: "session",
			run: func(ctx context.Context, repo Repository) error {
				_, err := repo.PurgeSession(ctx, "session-purge-send-now")
				return err
			},
		},
		{
			name: "task",
			run: func(ctx context.Context, repo Repository) error {
				_, err := repo.PurgeTask(ctx, "task-purge-send-now")
				return err
			},
		},
	} {
		for _, accepted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/accepted=%t", purge.name, accepted), func(t *testing.T) {
				repo := newTestSQLiteRepo(t)
				ctx := context.Background()
				entry := insertTestEntry(
					t, repo, "session-purge-send-now", "task-purge-send-now",
					"durable source", QueuedByUser, nil, nil,
				)
				pendingClaim, err := repo.ClaimSendNow(ctx, entry.SessionID, []QueuedMessage{*entry})
				if err != nil {
					t.Fatal(err)
				}
				persistent := repo.(pendingSendNowClaimRepository)
				if accepted {
					if err := persistent.MarkPendingSendNowClaimAccepted(ctx, pendingClaim); err != nil {
						t.Fatal(err)
					}
				}

				if err := purge.run(ctx, repo); err != nil {
					t.Fatal(err)
				}
				claims, err := persistent.ListPendingSendNowClaims(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(claims) != 0 {
					t.Fatalf("durable Send Now claims after purge = %#v, want none", claims)
				}
			})
		}
	}
}

func TestSQLiteSendNowAcknowledgeGenerationChangeRetiresClaim(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
	claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*first})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DeleteAllBySession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AcknowledgeSendNowClaim(ctx, claim); !errors.Is(err, ErrSendNowClaimChanged) {
		t.Fatalf("acknowledge after generation change error = %v, want %v", err, ErrSendNowClaimChanged)
	}

	second := insertTestEntry(t, repo, "session-1", "task-1", "second", QueuedByUser, nil, nil)
	if _, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*second}); err != nil {
		t.Fatalf("second same-process Send Now claim: %v", err)
	}
}

func TestSQLiteStaleSendNowWorkerCannotSettleSuccessorClaim(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(context.Context, *sqliteRepository, *SendNowClaim) error
	}{
		{
			name: "restore",
			run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
				return repo.RestoreSendNowClaim(ctx, claim)
			},
		},
		{
			name: "acknowledge",
			run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
				return repo.AcknowledgeSendNowClaim(ctx, claim)
			},
		},
		{
			name: "acknowledge-unidentified",
			run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
				unidentified := *claim
				unidentified.ClaimID = ""
				return repo.AcknowledgeSendNowClaim(ctx, &unidentified)
			},
		},
		{
			name: "mark-accepted",
			run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
				return repo.MarkPendingSendNowClaimAccepted(ctx, claim)
			},
		},
		{
			name: "startup-discard",
			run: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
				return repo.DeletePendingSendNowClaim(ctx, claim)
			},
		},
	} {
		t.Run(operation.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newTestSQLiteRepo(t).(*sqliteRepository)
			oldSource := insertTestEntry(t, repo, "session-stale-worker", "task-stale-worker", "old", QueuedByUser, nil, nil)
			oldClaim, err := repo.ClaimSendNow(ctx, oldSource.SessionID, []QueuedMessage{*oldSource})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.AcknowledgeSendNowClaim(ctx, oldClaim); err != nil {
				t.Fatal(err)
			}
			successorSource := insertTestEntry(
				t, repo, oldSource.SessionID, oldSource.TaskID, "successor", QueuedByUser, nil, nil,
			)
			successor, err := repo.ClaimSendNow(ctx, successorSource.SessionID, []QueuedMessage{*successorSource})
			if err != nil {
				t.Fatal(err)
			}

			if err := operation.run(ctx, repo, oldClaim); !errors.Is(err, ErrSendNowClaimChanged) {
				t.Fatalf("stale %s error = %v, want %v", operation.name, err, ErrSendNowClaimChanged)
			}
			pending, err := repo.ListPendingSendNowClaims(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 1 || pending[0].Claim.Dispatch.ID != successor.Dispatch.ID || pending[0].Accepted {
				t.Fatalf("successor after stale %s = %#v", operation.name, pending)
			}
			entries, err := repo.ListBySession(ctx, oldSource.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("queue after stale %s = %#v, want successor sources still claimed", operation.name, entries)
			}
		})
	}
}

func TestSQLiteSendNowClaimMigratesLegacyIdentity(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	_, err := repo.db.ExecContext(ctx, `
		CREATE TABLE queue_send_now_claims (
			session_id TEXT PRIMARY KEY,
			claim_json TEXT NOT NULL,
			accepted INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	legacy := SendNowClaim{
		Sources: []QueuedMessage{{
			ID: "legacy-source", SessionID: "legacy-session", TaskID: "legacy-task",
			Content: "legacy", QueuedBy: QueuedByUser,
		}},
		Dispatch: QueuedMessage{ID: "legacy-dispatch", SessionID: "legacy-session", TaskID: "legacy-task"},
	}
	claimJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO queue_send_now_claims (session_id, claim_json, accepted, created_at)
		VALUES (?, ?, 0, CURRENT_TIMESTAMP)
	`, legacy.Dispatch.SessionID, string(claimJSON)); err != nil {
		t.Fatal(err)
	}

	pending, err := repo.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Claim.ClaimID == "" {
		t.Fatalf("migrated Send Now claim = %#v", pending)
	}
	if err := repo.MarkPendingSendNowClaimAccepted(ctx, &pending[0].Claim); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeletePendingSendNowClaim(ctx, &pending[0].Claim); err != nil {
		t.Fatal(err)
	}
	pending, err = repo.ListPendingSendNowClaims(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending claims after migrated settlement = %#v, err=%v", pending, err)
	}
}
