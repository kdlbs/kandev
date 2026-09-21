package messagequeue

import (
	"context"
	"errors"
	"testing"
)

func TestSQLiteLegacyTransferRejectsIdentityAwareSendNowClaim(t *testing.T) {
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
	source := &QueuedMessage{
		ID:        "source",
		TaskID:    sourceIdentity.TaskID,
		SessionID: sourceIdentity.SessionID,
		Content:   "identity-aware source",
		QueuedBy:  QueuedByUser,
	}
	if err := repo.InsertForSession(ctx, sourceIdentity, source, DefaultMaxPerSession); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ClaimSendNowForSession(ctx, sourceIdentity, []QueuedMessage{*source}); err != nil {
		t.Fatal(err)
	}

	err := repo.TransferSession(ctx, sourceIdentity.SessionID, destinationIdentity.SessionID)
	if !errors.Is(err, ErrSessionIdentityMismatch) {
		t.Fatalf("legacy transfer error = %v, want ErrSessionIdentityMismatch", err)
	}
	pending, err := repo.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Claim.Identity != sourceIdentity {
		t.Fatalf("pending claims after rejected transfer = %#v, want source identity", pending)
	}
}
