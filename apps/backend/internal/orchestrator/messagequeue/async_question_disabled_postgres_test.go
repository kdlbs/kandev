package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestPostgresAsyncQuestionsDisabledRetiresOnlyUnsentQueueEntries(t *testing.T) {
	ctx := context.Background()
	repo := newTestPostgresRepo(t)
	persistent := repo.(*sqliteRepository)
	identity := QueueSessionIdentity{
		TaskID: "async-question-retirement-task", SessionID: "async-question-retirement-session",
		SessionIncarnationID: "async-question-retirement-incarnation",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 20, logger.Default())
	require.NoError(t, repo.InsertForSession(ctx, identity, &QueuedMessage{
		ID: "ordinary-before-async", TaskID: identity.TaskID, SessionID: identity.SessionID,
		Content: "ordinary first", QueuedBy: QueuedByUser,
	}, 20))
	questionMetadata := map[string]interface{}{MetadataAsyncQuestion: "postgres-question", MetadataAsyncQuestionNotification: true}
	queued, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "postgres-async-admission", "unsent async prompt", "", QueuedByAsyncQuestion,
		false, nil, questionMetadata, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, "postgres-async-admission", queued.ID)
	require.NoError(t, repo.InsertForSession(ctx, identity, &QueuedMessage{
		ID: "ordinary-after-async", TaskID: identity.TaskID, SessionID: identity.SessionID,
		Content: "ordinary second", QueuedBy: QueuedByUser,
	}, 20))

	disposition, admittedAt, err := service.AsyncQuestionAdmissionDisposition(ctx, "postgres-async-admission", "postgres-question")
	require.NoError(t, err)
	require.Equal(t, AsyncQuestionAdmissionQueued, disposition)
	require.False(t, admittedAt.IsZero())
	removed, err := service.RetireAsyncQuestionQueueAdmission(ctx, "postgres-async-admission", "postgres-question")
	require.NoError(t, err)
	require.True(t, removed)
	entries, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, []string{"ordinary-before-async", "ordinary-after-async"}, []string{entries[0].ID, entries[1].ID})
	disposition, retainedAt, err := service.AsyncQuestionAdmissionDisposition(ctx, "postgres-async-admission", "postgres-question")
	require.NoError(t, err)
	require.Equal(t, AsyncQuestionAdmissionSettled, disposition)
	require.True(t, admittedAt.Equal(retainedAt), "retirement keeps the original queue receipt timestamp")

	reservedIdentity := QueueSessionIdentity{
		TaskID: identity.TaskID, SessionID: "async-question-retained-session",
		SessionIncarnationID: "async-question-retained-incarnation",
	}
	seedQueueSessionIdentity(t, repo, reservedIdentity)
	retained, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, reservedIdentity, "postgres-retained-admission", "retained async prompt", "", QueuedByAsyncQuestion,
		false, nil, questionMetadata, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.NoError(t, repo.InsertForSession(ctx, reservedIdentity, &QueuedMessage{
		ID: "ordinary-after-retained-async", TaskID: reservedIdentity.TaskID, SessionID: reservedIdentity.SessionID,
		Content: "ordinary behind retained prompt", QueuedBy: QueuedByUser,
	}, 20))
	reserved, exists, autoRun, err := service.ReserveQueuedForDeliveryWithAutoRunForSession(ctx, reservedIdentity)
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, autoRun)
	require.Equal(t, retained.ID, reserved.ID)
	removed, err = service.RetireAsyncQuestionQueueAdmission(ctx, "postgres-retained-admission", "postgres-question")
	require.NoError(t, err)
	require.False(t, removed, "a reserved prompt keeps its delivery receipt until settlement")
	next, exists := service.TakeQueuedIfAutoRun(ctx, reservedIdentity.SessionID)
	require.True(t, exists, "the retained retired prompt must not block ordinary hand-off work")
	require.Equal(t, "ordinary-after-retained-async", next.ID)
	row := persistent.db.QueryRowxContext(ctx, persistent.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		  FROM queued_messages WHERE id = ? AND session_id = ?
	`), retained.ID, reservedIdentity.SessionID)
	stored, _, err := scanQueuedRowWithMetadataJSON(row)
	require.NoError(t, err)
	require.True(t, stored.IsReservedInFlight())
	require.Equal(t, true, stored.Metadata[MetadataAsyncQuestionRetired])
	require.NoError(t, repo.AcknowledgeReserved(ctx, reserved))
	var remaining int
	require.NoError(t, persistent.db.GetContext(ctx, &remaining, persistent.db.Rebind(
		`SELECT COUNT(*) FROM queued_messages WHERE id = ? AND session_id = ?`,
	), retained.ID, reservedIdentity.SessionID))
	require.Zero(t, remaining)
}
