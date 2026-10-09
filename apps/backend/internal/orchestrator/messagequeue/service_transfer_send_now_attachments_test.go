package messagequeue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDurableSessionTransferIncludesPendingSendNowClaimAttachments(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	source, err := service.QueueMessage(
		ctx,
		"session-old",
		"task",
		"handoff",
		"",
		QueuedByUser,
		false,
		[]MessageAttachment{{AttachmentID: "claim-attachment"}},
	)
	require.NoError(t, err)
	_, err = service.ClaimSendNow(ctx, source.SessionID, []QueuedMessage{*source})
	require.NoError(t, err)

	var preparedAttachments []string
	err = service.TransferSessionWithDurableAttachmentPreparation(
		ctx,
		"task",
		"session-old",
		"session-new",
		func(_ context.Context, attachmentIDs []string) error {
			preparedAttachments = append(preparedAttachments, attachmentIDs...)
			compensations, listErr := service.ListSessionTransferCompensations(ctx)
			require.NoError(t, listErr)
			require.Len(t, compensations, 1)
			require.Equal(t, []string{"claim-attachment"}, compensations[0].AttachmentIDs)
			return nil
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, []string{"claim-attachment"}, preparedAttachments)

	pending, err := service.ListPendingSendNowClaims(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "session-new", pending[0].Claim.Dispatch.SessionID)
	require.NoError(t, service.RestoreSendNowClaim(ctx, &pending[0].Claim))
	entries, err := repo.ListBySession(ctx, "session-new")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "claim-attachment", entries[0].Attachments[0].AttachmentID)
}
