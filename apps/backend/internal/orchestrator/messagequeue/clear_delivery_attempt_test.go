package messagequeue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func reserveAttemptedPlanComment(
	t *testing.T, repository Repository, identity QueueSessionIdentity,
) *QueuedMessage {
	t.Helper()
	ctx := context.Background()
	seedQueueSessionIdentity(t, repository, identity)
	require.NoError(t, repository.InsertForSession(
		ctx, identity, planCommentLeaseMessage(identity), DefaultMaxPerSession,
	))
	reserved, _, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.NotNil(t, reserved)
	require.NoError(t, repository.MarkDeliveryAttemptedForSession(ctx, identity, []QueuedMessage{*reserved}))
	return reserved
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestClearDeliveryAttemptRevokesOnlyTheMarker(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-clear", SessionID: "session-clear", SessionIncarnationID: "incarnation-clear",
			}
			reserved := reserveAttemptedPlanComment(t, repository, identity)

			require.NoError(t, repository.ClearDeliveryAttemptForSession(ctx, identity, []QueuedMessage{*reserved}))

			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.False(t, stored[0].IsDeliveryAttempted())
			require.True(t, stored[0].IsReservedInFlight(), "the reservation stays owned for the error path")
			recorded, _ := stored[0].Metadata[metadataUserMessageRecorded].(bool)
			require.True(t, recorded, "the transcript marker survives so a retry writes no second transcript")
			require.True(t, stored[0].IsDurablePlanComment(), "replay identity must survive")
		})
	}
}

func TestClearDeliveryAttemptRejectsStaleOrForeignEntries(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-reject", SessionID: "session-reject", SessionIncarnationID: "incarnation-reject",
			}
			reserved := reserveAttemptedPlanComment(t, repository, identity)

			stale := identity
			stale.SessionIncarnationID = "replaced-incarnation"
			require.Error(t, repository.ClearDeliveryAttemptForSession(ctx, stale, []QueuedMessage{*reserved}))

			ordinary := *reserved
			ordinary.Metadata = map[string]interface{}{}
			require.ErrorIs(t, repository.ClearDeliveryAttemptForSession(ctx, identity, []QueuedMessage{ordinary}), ErrEntryNotFound)
			require.ErrorIs(t, repository.ClearDeliveryAttemptForSession(ctx, identity, nil), ErrEntryNotFound)

			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.True(t, stored[0].IsDeliveryAttempted(), "a rejected clear leaves the marker in place")

			foreignOwner := *reserved
			foreignOwner.reservationToken = "another-owner"
			require.ErrorIs(t, repository.ClearDeliveryAttemptForSession(ctx, identity, []QueuedMessage{foreignOwner}), ErrEntryNotFound,
				"only the reservation owner can clear the marker")

			require.NoError(t, repository.ClearDeliveryAttemptForSession(ctx, identity, []QueuedMessage{*reserved}))
			require.ErrorIs(t, repository.ClearDeliveryAttemptForSession(ctx, identity, []QueuedMessage{*reserved}), ErrEntryNotFound,
				"an entry without the marker cannot be cleared twice")
		})
	}
}

func TestClearDeliveryAttemptIsAllOrNothing(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-atomic", SessionID: "session-atomic", SessionIncarnationID: "incarnation-atomic",
			}
			reserved := reserveAttemptedPlanComment(t, repository, identity)
			foreignOwner := *reserved
			foreignOwner.ID = "missing-entry"

			require.ErrorIs(t, repository.ClearDeliveryAttemptForSession(
				ctx, identity, []QueuedMessage{*reserved, foreignOwner},
			), ErrEntryNotFound)
			require.ErrorIs(t, repository.ClearDeliveryAttemptForSession(
				ctx, identity, []QueuedMessage{*reserved, *reserved},
			), ErrQueueChanged)

			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.True(t, stored[0].IsDeliveryAttempted(), "a failed batch clears nothing")
		})
	}
}

func TestPostgresClearDeliveryAttemptRevokesOnlyTheMarker(t *testing.T) {
	ctx := context.Background()
	repository, _, _ := newTestPostgresRepoPair(t)
	identity := QueueSessionIdentity{
		TaskID: "task-pg-clear", SessionID: "session-pg-clear", SessionIncarnationID: "incarnation-pg-clear",
	}
	reserved := reserveAttemptedPlanComment(t, repository, identity)
	require.NoError(t, repository.ClearDeliveryAttemptForSession(ctx, identity, []QueuedMessage{*reserved}))
	stored, err := repository.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.False(t, stored[0].IsDeliveryAttempted())
}
