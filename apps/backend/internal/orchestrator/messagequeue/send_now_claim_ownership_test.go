package messagequeue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/stretchr/testify/require"
)

// planCommentSendNowSource builds the durable plan-comment row that stays in
// queued_messages while a Send Now claim owns it.
func planCommentSendNowSource(sessionID, taskID, entryID string) *QueuedMessage {
	return &QueuedMessage{
		ID: entryID, SessionID: sessionID, TaskID: taskID,
		Content: "plan comment feedback", QueuedBy: QueuedByUser,
		Metadata: map[string]interface{}{
			plancomments.MetadataClientQueueID:      "client-" + entryID,
			plancomments.MetadataRequestFingerprint: "fingerprint-" + entryID,
		},
	}
}

// expirePlanCommentLease rewrites the stored reservation expiry into the past,
// leaving the reservation token untouched. A claim still owns the row even
// after this lease expires.
func expirePlanCommentLease(t *testing.T, repo Repository, sessionID, entryID string) {
	t.Helper()
	expiredAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	switch typed := repo.(type) {
	case *memoryRepository:
		typed.mu.Lock()
		defer typed.mu.Unlock()
		for _, entry := range typed.entries[sessionID] {
			if entry.ID == entryID {
				entry.Metadata[MetadataDeliveryReservationExpiresAt] = expiredAt
				return
			}
		}
		t.Fatalf("queued entry %s not found in memory repository", entryID)
	case *sqliteRepository:
		expireSQLitePlanCommentLease(t, typed, sessionID, entryID, expiredAt)
	default:
		t.Fatalf("unsupported repository %T", repo)
	}
}

func expireSQLitePlanCommentLease(
	t *testing.T,
	repo *sqliteRepository,
	sessionID, entryID, expiredAt string,
) {
	t.Helper()
	ctx := context.Background()
	var raw string
	require.NoError(t, repo.db.QueryRowxContext(ctx, repo.db.Rebind(`
		SELECT metadata_json FROM queued_messages WHERE id = ? AND session_id = ?
	`), entryID, sessionID).Scan(&raw))
	var metadata map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &metadata))
	metadata[MetadataDeliveryReservationExpiresAt] = expiredAt
	updated, err := json.Marshal(metadata)
	require.NoError(t, err)
	result, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE queued_messages SET metadata_json = ? WHERE id = ? AND session_id = ?
	`), string(updated), entryID, sessionID)
	require.NoError(t, err)
	affected, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)
}

func pendingSendNowClaims(t *testing.T, repo Repository) int {
	t.Helper()
	switch typed := repo.(type) {
	case *memoryRepository:
		typed.mu.Lock()
		defer typed.mu.Unlock()
		total := 0
		for _, entries := range typed.sendNowClaims {
			total += len(entries)
		}
		return total
	default:
		persistent, ok := repo.(interface {
			ListPendingSendNowClaims(context.Context) ([]PendingSendNowClaim, error)
		})
		require.True(t, ok, "%T must persist pending Send Now claims", repo)
		pending, err := persistent.ListPendingSendNowClaims(context.Background())
		require.NoError(t, err)
		return len(pending)
	}
}

// TestSendNowClaimOwnsItsSourcesAfterLeaseExpiry covers the Send Now incident:
// the replacement prompt outlived the 30-second delivery lease, FIFO then
// reserved the claimed durable row, and the claim's later restore failed with
// ErrSendNowClaimChanged. FIFO must not reserve a row an open claim owns.
func TestSendNowClaimOwnsItsSourcesAfterLeaseExpiry(t *testing.T) {
	factories := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}
	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repo := factory.new(t)
			source := planCommentSendNowSource("session-1", "task-1", "plan-comment-1")
			require.NoError(t, repo.Insert(ctx, source, 0))
			next := &QueuedMessage{
				ID: "next-entry", SessionID: "session-1", TaskID: "task-1",
				Content: "next prompt", QueuedBy: QueuedByUser,
			}
			require.NoError(t, repo.Insert(ctx, next, 0))

			claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*source})
			require.NoError(t, err)
			require.Equal(t, 1, pendingSendNowClaims(t, repo), "claim must be durable")

			expirePlanCommentLease(t, repo, "session-1", source.ID)

			reserved, err := repo.ReserveHead(ctx, "session-1")
			require.NoError(t, err)
			require.Nil(t, reserved, "FIFO must not take a row the open claim owns")

			pending, err := repo.ListBySession(ctx, "session-1")
			require.NoError(t, err)
			require.Len(t, pending, 2, "claim ownership must not skip to the next entry")
			require.Equal(t, source.ID, pending[0].ID)
			require.Equal(t, next.ID, pending[1].ID)

			require.NoError(t, repo.RestoreSendNowClaim(ctx, claim))
			require.Zero(t, pendingSendNowClaims(t, repo), "restore must release the claim")

			restored, err := repo.ListBySession(ctx, "session-1")
			require.NoError(t, err)
			require.Len(t, restored, 2)
			require.Equal(t, source.ID, restored[0].ID)
			require.False(t, restored[0].IsReservedInFlight(), "restore must clear the claim reservation")
			require.Equal(t, next.ID, restored[1].ID)
		})
	}
}

// TestUnownedPlanCommentLeaseStillExpires pins that ordinary FIFO reservations
// keep their existing lease behavior when no Send Now claim owns the row.
func TestUnownedPlanCommentLeaseStillExpires(t *testing.T) {
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
			source := planCommentSendNowSource("session-1", "task-1", "unowned-plan-comment")
			source.Metadata = markReservedMetadataWithLeaseForTest(source.Metadata, time.Now())
			require.NoError(t, repo.Insert(ctx, source, 0))

			reserved, err := repo.ReserveHead(ctx, "session-1")
			require.NoError(t, err)
			require.Nil(t, reserved, "a live plan-comment lease must still block FIFO")

			expirePlanCommentLease(t, repo, "session-1", source.ID)
			reserved, err = repo.ReserveHead(ctx, "session-1")
			require.NoError(t, err)
			require.NotNil(t, reserved, "an expired lease without a claim must become reservable")
			require.Equal(t, source.ID, reserved.ID)
		})
	}
}

func markReservedMetadataWithLeaseForTest(
	metadata map[string]interface{},
	now time.Time,
) map[string]interface{} {
	marked, _, _ := markReservedMetadataWithLease(metadata, "", now)
	return marked
}

// TestReleaseSendNowClaimSettlesOwnedSources pins that releasing an
// unrestorable claim removes its surviving source rows, so FIFO cannot deliver
// a message already reported as undelivered.
func TestReleaseSendNowClaimSettlesOwnedSources(t *testing.T) {
	factories := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}
	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repo := factory.new(t)
			source := planCommentSendNowSource("session-1", "task-1", "plan-comment-release")
			require.NoError(t, repo.Insert(ctx, source, 0))
			claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*source})
			require.NoError(t, err)

			settled, err := repo.ReleaseSendNowClaim(ctx, claim)
			require.NoError(t, err)
			require.True(t, settled)
			entries, err := repo.ListBySession(ctx, "session-1")
			require.NoError(t, err)
			require.Empty(t, entries)
			require.Zero(t, pendingSendNowClaims(t, repo))
		})
	}
}

// TestReleaseSendNowClaimLeavesSuccessorOwnership pins that a claim superseded
// by another claim does not remove the successor's source or report a settled
// outcome.
func TestReleaseSendNowClaimLeavesSuccessorOwnership(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := planCommentSendNowSource("session-1", "task-1", "plan-comment-successor")
	require.NoError(t, repo.Insert(ctx, source, 0))
	claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*source})
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx,
		`UPDATE queue_send_now_claims SET claim_id = ? WHERE session_id = ?`, "successor-claim", "session-1")
	require.NoError(t, err)

	settled, err := repo.ReleaseSendNowClaim(ctx, claim)
	require.NoError(t, err)
	require.False(t, settled)
	entries, err := repo.ListBySession(ctx, "session-1")
	require.NoError(t, err)
	require.Len(t, entries, 1, "the successor's source must remain")
	require.Equal(t, 1, pendingSendNowClaims(t, repo))
}

// TestMemorySendNowKeepsEarlierClaimOwnership pins that a later claim adds its
// own sources without clearing an earlier claim's durable ownership. The later
// ordinary-only claim must not make the first claim's source reservable.
func TestMemorySendNowKeepsEarlierClaimOwnership(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	first := planCommentSendNowSource("session-1", "task-1", "plan-comment-first")
	require.NoError(t, repo.Insert(ctx, first, 0))
	firstClaim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*first})
	require.NoError(t, err)

	second := &QueuedMessage{
		ID: "ordinary-second", SessionID: "session-1", TaskID: "task-1",
		Content: "second send now", QueuedBy: QueuedByUser,
	}
	require.NoError(t, repo.Insert(ctx, second, 0))
	_, err = repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*second})
	require.NoError(t, err)

	reserved, err := repo.ReserveHead(ctx, "session-1")
	require.NoError(t, err)
	require.Nil(t, reserved, "the first claim must still own its durable source")

	settled, err := repo.ReleaseSendNowClaim(ctx, firstClaim)
	require.NoError(t, err)
	require.True(t, settled)
	entries, err := repo.ListBySession(ctx, "session-1")
	require.NoError(t, err)
	require.Empty(t, entries)
}
