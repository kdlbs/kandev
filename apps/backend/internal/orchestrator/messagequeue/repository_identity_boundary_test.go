package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/stretchr/testify/require"
)

func TestServiceExactTargetedOperationsRejectReplacementSession(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			queue := NewService(repository, DefaultMaxPerSession, logger.Default())
			first := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repository, first)

			queued, _, err := queue.QueueMessageWithCoalesceKeyForSession(
				ctx, first, "original", "", QueuedByAgent, false, nil,
				map[string]interface{}{MetadataCoalesceKey: "automation"}, "automation", true,
			)
			require.NoError(t, err)

			replacement := first
			replacement.SessionIncarnationID = "incarnation-2"
			seedQueueSessionIdentity(t, repository, replacement)

			_, _, err = queue.TakeQueuedEntryForSession(ctx, first, queued.ID)
			require.ErrorIs(t, err, ErrSessionIdentityMismatch)
			_, _, err = queue.QueueMessageWithCoalesceKeyForSession(
				ctx, first, "stale replacement", "", QueuedByAgent, false, nil,
				nil, "automation", true,
			)
			require.ErrorIs(t, err, ErrSessionIdentityMismatch)
		})
	}
}

func TestRepositoryRetainedDeliveryReservationLifecycle(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repository, identity)
			require.NoError(t, repository.InsertForSession(ctx, identity, &QueuedMessage{
				ID: "queued-1", TaskID: identity.TaskID, SessionID: identity.SessionID, Content: "prompt",
			}, DefaultMaxPerSession))

			reserved, autoRun, err := repository.ReserveHeadForDeliveryIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.True(t, autoRun)
			require.NotNil(t, reserved)
			require.False(t, reserved.IsReservedInFlight())
			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.True(t, stored[0].IsReservedInFlight())

			require.NoError(t, repository.ReleaseDeliveryReservationForSession(ctx, identity, reserved))
			stored, err = repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.False(t, stored[0].IsReservedInFlight())

			reserved, autoRun, err = repository.ReserveHeadForDeliveryIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.True(t, autoRun)
			require.NoError(t, repository.AcknowledgeByIDForSession(ctx, identity, reserved))
			stored, err = repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Empty(t, stored)
		})
	}
}

func TestRepositoryPlanCommentReservationLeaseBlocksConcurrentOwner(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-lease", SessionID: "session-lease", SessionIncarnationID: "incarnation-lease",
			}
			seedQueueSessionIdentity(t, repository, identity)
			require.NoError(t, repository.InsertForSession(ctx, identity, planCommentLeaseMessage(identity), DefaultMaxPerSession))

			first, autoRun, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.True(t, autoRun)
			require.NotNil(t, first)
			require.NotEmpty(t, first.reservationToken)

			second, autoRun, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.True(t, autoRun)
			require.Nil(t, second)
			require.NoError(t, repository.ReleaseDeliveryReservationForSession(ctx, identity, first))
		})
	}
}

func TestRepositoryExpiredPlanCommentLeaseRejectsStaleOwner(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-expired", SessionID: "session-expired", SessionIncarnationID: "incarnation-expired",
			}
			seedQueueSessionIdentity(t, repository, identity)
			message := planCommentLeaseMessage(identity)
			expiredMetadata, _, _ := markReservedMetadataWithLease(
				message.Metadata, identity.SessionIncarnationID, time.Now().Add(-DeliveryReservationTTL-time.Minute),
			)
			message.Metadata = expiredMetadata
			require.NoError(t, repository.InsertForSession(ctx, identity, message, DefaultMaxPerSession))

			stale := *message
			stale.reservedDelivery = true
			stale.reservationIdentity = identity
			stale.bindDeliveryReservation(expiredMetadata)
			current, _, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.NotNil(t, current)
			require.NotEqual(t, stale.reservationToken, current.reservationToken)

			require.ErrorIs(t, repository.AcknowledgeByIDForSession(ctx, identity, &stale), ErrEntryNotFound)
			require.NoError(t, repository.MarkDeliveryAttemptedForSession(ctx, identity, []QueuedMessage{*current}))
			require.NoError(t, repository.AcknowledgeByIDForSession(ctx, identity, current))
		})
	}
}

func TestPostgresPlanCommentReservationLeaseBlocksSecondRepository(t *testing.T) {
	ctx := context.Background()
	repositoryA, repositoryB, _ := newTestPostgresRepoPair(t)
	identity := QueueSessionIdentity{
		TaskID: "task-pg-lease", SessionID: "session-pg-lease", SessionIncarnationID: "incarnation-pg-lease",
	}
	seedQueueSessionIdentity(t, repositoryA, identity)
	seedQueueSessionIdentity(t, repositoryB, identity)
	require.NoError(t, repositoryA.InsertForSession(ctx, identity, planCommentLeaseMessage(identity), DefaultMaxPerSession))

	first, _, err := repositoryA.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.NotNil(t, first)
	second, _, err := repositoryB.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.Nil(t, second)
	require.NoError(t, repositoryA.ReleaseDeliveryReservationForSession(ctx, identity, first))
}

func TestSQLitePlanCommentDeliveryAttemptRejectsArchivedTask(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	identity := QueueSessionIdentity{
		TaskID: "task-archived-delivery", SessionID: "session-archived-delivery",
		SessionIncarnationID: "incarnation-archived-delivery",
	}
	seedQueueSessionIdentity(t, repository, identity)
	require.NoError(t, repository.InsertForSession(
		ctx, identity, planCommentLeaseMessage(identity), DefaultMaxPerSession,
	))
	reserved, _, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.NotNil(t, reserved)

	sqliteRepo := repository.(*sqliteRepository)
	_, err = sqliteRepo.db.ExecContext(
		ctx, `UPDATE tasks SET archived_at = ? WHERE id = ?`, time.Now().UTC(), identity.TaskID,
	)
	require.NoError(t, err)
	require.ErrorIs(
		t, repository.MarkDeliveryAttemptedForSession(ctx, identity, []QueuedMessage{*reserved}),
		ErrTaskInactive,
	)
	stored, err := repository.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.False(t, stored[0].IsDeliveryAttempted())
}

func TestPostgresPlanCommentDeliveryAttemptRejectsArchivedTask(t *testing.T) {
	ctx := context.Background()
	repository, _, database := newTestPostgresRepoPair(t)
	identity := QueueSessionIdentity{
		TaskID: "task-pg-archived-delivery", SessionID: "session-pg-archived-delivery",
		SessionIncarnationID: "incarnation-pg-archived-delivery",
	}
	seedQueueSessionIdentity(t, repository, identity)
	require.NoError(t, repository.InsertForSession(
		ctx, identity, planCommentLeaseMessage(identity), DefaultMaxPerSession,
	))
	reserved, _, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.NotNil(t, reserved)
	_, err = database.ExecContext(
		ctx, `UPDATE tasks SET archived_at = $1 WHERE id = $2`, time.Now().UTC(), identity.TaskID,
	)
	require.NoError(t, err)
	require.ErrorIs(
		t, repository.MarkDeliveryAttemptedForSession(ctx, identity, []QueuedMessage{*reserved}),
		ErrTaskInactive,
	)
}

func planCommentLeaseMessage(identity QueueSessionIdentity) *QueuedMessage {
	return &QueuedMessage{
		ID: "queued-plan-comment", TaskID: identity.TaskID, SessionID: identity.SessionID, Content: "feedback",
		Metadata: map[string]interface{}{
			plancomments.MetadataClientQueueID:      "caller-plan-comment",
			plancomments.MetadataRequestFingerprint: "fingerprint-plan-comment",
		},
	}
}

func TestRepositoryLifecycleCoalescingPreservesInFlightReservation(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repository, identity)
			message := func(id, content string) *QueuedMessage {
				return &QueuedMessage{
					ID: id, TaskID: identity.TaskID, SessionID: identity.SessionID,
					Content: content, QueuedBy: QueuedByWorkflow,
					Metadata: map[string]interface{}{
						MetadataLifecycleDurable:    true,
						MetadataLifecycleGeneration: float64(0),
						MetadataCoalesceKey:         "lifecycle:event",
					},
				}
			}
			_, replaced, err := repository.InsertOrReplaceLifecycleByCoalesceKeyForSession(
				ctx, identity, message("reserved", "first"), "lifecycle:event", 1, true,
			)
			require.NoError(t, err)
			require.False(t, replaced)
			reserved, _, err := repository.ReserveHeadForDeliveryIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.Equal(t, "reserved", reserved.ID)

			successor, replaced, err := repository.InsertOrReplaceLifecycleByCoalesceKeyForSession(
				ctx, identity, message("successor", "second"), "lifecycle:event", 1, true,
			)
			require.NoError(t, err)
			require.False(t, replaced)
			require.Equal(t, "successor", successor.ID)

			successor, replaced, err = repository.InsertOrReplaceLifecycleByCoalesceKeyForSession(
				ctx, identity, message("replacement", "third"), "lifecycle:event", 1, true,
			)
			require.NoError(t, err)
			require.True(t, replaced)
			require.Equal(t, "successor", successor.ID)
			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 2)
			require.Equal(t, "first", stored[0].Content)
			require.True(t, stored[0].IsReservedInFlight())
			require.Equal(t, "third", stored[1].Content)
			require.False(t, stored[1].IsReservedInFlight())

			require.NoError(t, repository.RequeuePreservingFIFOForSession(ctx, identity, reserved))
			stored, err = repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.Equal(t, "successor", stored[0].ID)
			require.Equal(t, "third", stored[0].Content)
			require.False(t, stored[0].IsReservedInFlight())
		})
	}
}

func TestRepositoryCoalescingReleasePrefersPendingSuccessor(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repository, identity)
			message := func(id, content string) *QueuedMessage {
				return &QueuedMessage{
					ID: id, TaskID: identity.TaskID, SessionID: identity.SessionID,
					Content: content, QueuedBy: QueuedByAgent,
					Metadata: map[string]interface{}{MetadataCoalesceKey: "automation"},
				}
			}
			_, replaced, err := repository.InsertOrReplaceByCoalesceKeyForSession(
				ctx, identity, message("reserved", "first"), "automation", 1, true,
			)
			require.NoError(t, err)
			require.False(t, replaced)
			reserved, _, err := repository.ReserveHeadForDeliveryIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)

			_, replaced, err = repository.InsertOrReplaceByCoalesceKeyForSession(
				ctx, identity, message("successor", "second"), "automation", 1, true,
			)
			require.NoError(t, err)
			require.False(t, replaced)

			require.NoError(t, repository.ReleaseDeliveryReservationForSession(ctx, identity, reserved))
			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.Equal(t, "successor", stored[0].ID)
			require.False(t, stored[0].IsReservedInFlight())
		})
	}
}

func TestRepositoryRetainedOrdinaryReservationRejectsReplacementSession(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			first := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repository, first)
			require.NoError(t, repository.InsertForSession(ctx, first, &QueuedMessage{
				ID: "queued-1", TaskID: first.TaskID, SessionID: first.SessionID, Content: "stale",
			}, DefaultMaxPerSession))
			reserved, _, err := repository.ReserveHeadForDeliveryIfAutoRunForSession(ctx, first)
			require.NoError(t, err)
			require.NotNil(t, reserved)

			replacement := first
			replacement.SessionIncarnationID = "incarnation-2"
			seedQueueSessionIdentity(t, repository, replacement)
			reserved, _, err = repository.ReserveHeadForDeliveryIfAutoRunForSession(ctx, replacement)
			require.NoError(t, err)
			require.Nil(t, reserved)
			stored, err := repository.ListBySession(ctx, replacement.SessionID)
			require.NoError(t, err)
			require.Empty(t, stored)
		})
	}
}

func TestMemoryRepositoryAuthorityReplacementClearsSessionState(t *testing.T) {
	ctx := context.Background()
	current := QueueSessionIdentity{
		TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
	}
	repository := NewMemoryRepositoryWithAuthority(
		func(context.Context, string, string) (QueueSessionIdentity, error) {
			return current, nil
		},
	).(*memoryRepository)
	first, err := repository.ResolveSessionIdentity(ctx, current.TaskID, current.SessionID)
	require.NoError(t, err)
	require.NoError(t, repository.InsertForSession(ctx, first, &QueuedMessage{
		ID: "queued-1", TaskID: first.TaskID, SessionID: first.SessionID, Content: "stale",
	}, DefaultMaxPerSession))
	repository.mu.Lock()
	repository.pendingMoves[first.SessionID] = &PendingMove{
		MoveID: "move-1", TaskID: first.TaskID, SessionIncarnationID: first.SessionIncarnationID,
	}
	repository.mu.Unlock()

	current = QueueSessionIdentity{
		TaskID: "task-2", SessionID: "session-1", SessionIncarnationID: "incarnation-2",
	}
	replacement, err := repository.ResolveSessionIdentity(ctx, current.TaskID, current.SessionID)
	require.NoError(t, err)
	snapshot, err := repository.Snapshot(ctx, replacement)
	require.NoError(t, err)
	require.Empty(t, snapshot.Entries)
	require.Nil(t, snapshot.PendingMove)
}

func TestMemoryRepositoryPurgeTaskClearsPendingMoves(t *testing.T) {
	repository := NewMemoryRepository().(*memoryRepository)
	repository.mu.Lock()
	repository.pendingMoves["session-1"] = &PendingMove{
		MoveID: "move-1", TaskID: "task-1", SessionIncarnationID: "incarnation-1",
	}
	repository.mu.Unlock()

	_, err := repository.PurgeTask(context.Background(), "task-1")
	require.NoError(t, err)
	repository.mu.Lock()
	defer repository.mu.Unlock()
	require.NotContains(t, repository.pendingMoves, "session-1")
}

func TestRepositoryRetainedDeliveryReservationRejectsUserMutation(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repository, identity)
			require.NoError(t, repository.InsertForSession(ctx, identity, &QueuedMessage{
				ID: "queued-1", TaskID: identity.TaskID, SessionID: identity.SessionID,
				Content: "original", QueuedBy: QueuedByUser,
			}, DefaultMaxPerSession))
			reserved, _, err := repository.ReserveHeadForDeliveryIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.NotNil(t, reserved)

			err = repository.UpdateContentAndMetadataForSession(
				ctx, identity, reserved.ID, "edited", nil, nil, QueuedByUser,
			)
			require.ErrorIs(t, err, ErrEntryNotFound)
			_, err = repository.DeleteByIDForSession(ctx, identity, reserved.ID)
			require.ErrorIs(t, err, ErrEntryNotFound)

			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.Equal(t, "original", stored[0].Content)
			require.True(t, stored[0].IsReservedInFlight())
		})
	}
}

func TestRepositoryIdentityRestoreRejectsForeignTaskEntries(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repository, identity)

			err := repository.ReplaceSessionForIdentity(ctx, identity, []QueuedMessage{{
				ID: "foreign", TaskID: "task-2", SessionID: identity.SessionID, Content: "foreign",
			}}, nil)
			require.ErrorIs(t, err, ErrSessionIdentityMismatch)

			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Empty(t, stored)
		})
	}
}

func TestRepositoriesTransferSessionIdentitiesRejectCrossTaskStateMove(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			source := QueueSessionIdentity{
				TaskID: "task-source", SessionID: "session-source", SessionIncarnationID: "incarnation-source",
			}
			destination := QueueSessionIdentity{
				TaskID: "task-destination", SessionID: "session-destination", SessionIncarnationID: "incarnation-destination",
			}
			seedQueueSessionIdentity(t, repository, source)
			seedQueueSessionIdentity(t, repository, destination)

			require.NoError(t, repository.InsertForSession(ctx, source, &QueuedMessage{
				ID: "source-entry", TaskID: source.TaskID, SessionID: source.SessionID, Content: "source",
			}, DefaultMaxPerSession))
			require.NoError(t, repository.InsertForSession(ctx, destination, &QueuedMessage{
				ID: "destination-entry", TaskID: destination.TaskID, SessionID: destination.SessionID, Content: "destination",
			}, DefaultMaxPerSession))
			require.NoError(t, repository.SetPendingMove(ctx, source.SessionID, &PendingMove{
				MoveID:               "move-source",
				SessionIncarnationID: source.SessionIncarnationID,
				TaskID:               source.TaskID,
				WorkflowID:           "workflow-source",
				WorkflowStepID:       "step-source",
			}))
			require.NoError(t, repository.SetAutoRunForSession(ctx, source, false))
			_, err := repository.SetAutoMergeOverride(ctx, source, true)
			require.NoError(t, err)
			_, err = repository.SetAutoMergeOverride(ctx, destination, false)
			require.NoError(t, err)

			sourceBefore, err := repository.Snapshot(ctx, source)
			require.NoError(t, err)
			destinationBefore, err := repository.Snapshot(ctx, destination)
			require.NoError(t, err)
			sourceBefore.StatusGeneration = 0
			destinationBefore.StatusGeneration = 0

			err = repository.TransferSessionIdentities(ctx, source, destination)
			require.ErrorIs(t, err, ErrSessionIdentityMismatch)

			sourceAfter, err := repository.Snapshot(ctx, source)
			require.NoError(t, err)
			destinationAfter, err := repository.Snapshot(ctx, destination)
			require.NoError(t, err)
			sourceAfter.StatusGeneration = 0
			destinationAfter.StatusGeneration = 0
			require.Equal(t, sourceBefore, sourceAfter)
			require.Equal(t, destinationBefore, destinationAfter)
		})
	}
}
