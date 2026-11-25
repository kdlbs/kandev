package messagequeue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestAsyncQuestionQueueActorIsReserved(t *testing.T) {
	require.True(t, IsReservedQueuedBy(QueuedByAsyncQuestion))
	require.False(t, IsReservedQueuedBy(QueuedByUser))
}

func TestAsyncQuestionRetirementIgnoresOtherScopesWithSameClientID(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	ordinaryIdentity := QueueSessionIdentity{TaskID: "shared-receipt-task", SessionID: "ordinary-receipt-session", SessionIncarnationID: "ordinary-inc"}
	asyncIdentity := QueueSessionIdentity{TaskID: "shared-receipt-task", SessionID: "async-receipt-session", SessionIncarnationID: "async-inc"}
	seedQueueSessionIdentity(t, repo, ordinaryIdentity)
	seedQueueSessionIdentity(t, repo, asyncIdentity)
	service := NewService(repo, 10, logger.Default())
	const sharedClientID = "same-client-id"
	ordinaryAccepted := &QueuedMessage{
		ID: "ordinary-fold-target", TaskID: ordinaryIdentity.TaskID, SessionID: ordinaryIdentity.SessionID,
		Content: "ordinary folded entry", QueuedBy: QueuedByUser,
	}
	require.NoError(t, repo.InsertForSession(ctx, ordinaryIdentity, ordinaryAccepted, 10))
	responseJSON, err := json.Marshal(ordinaryAccepted)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO queue_admission_receipts
			(task_id, session_id, session_incarnation_id, client_queue_id, request_fingerprint,
			 accepted_queue_id, response_json, admitted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), ordinaryIdentity.TaskID, ordinaryIdentity.SessionID, ordinaryIdentity.SessionIncarnationID,
		sharedClientID, "ordinary-fold-fingerprint", ordinaryAccepted.ID, string(responseJSON), time.Now().UTC())
	require.NoError(t, err)
	disposition, _, err := service.AsyncQuestionAdmissionDisposition(ctx, sharedClientID, "question-1")
	require.NoError(t, err)
	require.Equal(t, AsyncQuestionAdmissionMissing, disposition, "an ordinary receipt in another scope is not async work")

	metadata := map[string]interface{}{MetadataAsyncQuestion: "question-1", MetadataAsyncQuestionNotification: true}
	_, _, err = service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, asyncIdentity, sharedClientID, "async entry", "", QueuedByAsyncQuestion, false, nil, metadata, nil,
	)
	require.NoError(t, err)
	disposition, _, err = service.AsyncQuestionAdmissionDisposition(ctx, sharedClientID, "question-1")
	require.NoError(t, err)
	require.Equal(t, AsyncQuestionAdmissionQueued, disposition)

	removed, err := service.RetireAsyncQuestionQueueAdmission(ctx, sharedClientID, "question-1")
	require.NoError(t, err)
	require.True(t, removed)
	ordinary, err := repo.ListBySession(ctx, ordinaryIdentity.SessionID)
	require.NoError(t, err)
	require.Len(t, ordinary, 1)
	require.Equal(t, QueuedByUser, ordinary[0].QueuedBy)
	async, err := repo.ListBySession(ctx, asyncIdentity.SessionID)
	require.NoError(t, err)
	require.Empty(t, async)
}

func TestSQLiteRetiredAsyncQuestionReservationSkipsFuturePrompts(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	identity := QueueSessionIdentity{
		TaskID: "retired-question-task", SessionID: "retired-question-session",
		SessionIncarnationID: "retired-question-incarnation",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())

	question, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-async-question", "question prompt", "", QueuedByAsyncQuestion,
		false, nil, map[string]interface{}{MetadataAsyncQuestion: "question-1"}, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	ordinary, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-ordinary-prompt", "ordinary prompt", "", QueuedByUser,
		false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	ordinaryAfter := &QueuedMessage{
		ID: "ordinary-prompt-after", SessionID: identity.SessionID, TaskID: identity.TaskID,
		Content: "ordinary prompt after", QueuedBy: QueuedByUser,
		Metadata: map[string]interface{}{MetadataAsyncQuestionRetired: true},
	}
	require.NoError(t, repo.InsertForSession(ctx, identity, ordinaryAfter, 10))

	reserved, exists, autoRun, err := service.ReserveQueuedForDeliveryWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, autoRun)
	require.True(t, exists)
	require.Equal(t, question.ID, reserved.ID)
	require.True(t, reserved.IsReservedDelivery())

	removed, err := repo.RetireAsyncQuestionQueueAdmission(ctx, "client-async-question", "question-1")
	require.NoError(t, err)
	require.False(t, removed, "an in-flight row remains for its delivery receipt to settle")

	var storedMetadataJSON string
	require.NoError(t, repo.db.GetContext(ctx, &storedMetadataJSON, repo.db.Rebind(
		`SELECT metadata_json FROM queued_messages WHERE id = ?`,
	), question.ID))
	storedMetadata := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(storedMetadataJSON), &storedMetadata))
	require.Equal(t, true, storedMetadata[MetadataAsyncQuestionRetired])
	require.Equal(t, true, storedMetadata[MetadataLifecycleReserved])
	require.NotEmpty(t, storedMetadata[metadataLifecycleReservationID])

	byID, taken, err := service.TakeQueuedEntry(ctx, identity.SessionID, question.ID)
	require.NoError(t, err)
	require.False(t, taken, "a retired reservation must not be taken by id")
	require.Nil(t, byID)

	byHead, taken := service.TakeQueuedIfAutoRun(ctx, identity.SessionID)
	require.True(t, taken, "the ordinary prompt behind the retired reservation remains drainable")
	require.Equal(t, ordinary.ID, byHead.ID)

	next, exists, autoRun, err := service.ReserveQueuedForDeliveryWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, autoRun)
	require.True(t, exists)
	require.Equal(t, ordinaryAfter.ID, next.ID, "the retired reserved row must not block ordinary work")
	require.NoError(t, repo.AcknowledgeReserved(ctx, next))

	next, exists, _, err = service.ReserveQueuedForDeliveryWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.False(t, exists, "the retired row must remain ineligible while its delivery is unresolved")
	require.Nil(t, next)

	require.NoError(t, repo.AcknowledgeReserved(ctx, reserved))
	var count int
	require.NoError(t, repo.db.GetContext(ctx, &count, repo.db.Rebind(
		`SELECT COUNT(*) FROM queued_messages WHERE id = ?`,
	), question.ID))
	require.Zero(t, count, "the original delivery receipt can still acknowledge the retained row")
}
