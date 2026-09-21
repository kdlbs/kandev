package messagequeue

import (
	"context"
	"errors"
	"fmt"
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

func TestDurableTransferRejectsIdentityAwareClaimWhenDestinationIdentityIsUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	sourceIdentity := QueueSessionIdentity{
		TaskID:               "task-1",
		SessionID:            "session-old",
		SessionIncarnationID: "incarnation-old",
	}
	destinationIdentity := QueueSessionIdentity{TaskID: sourceIdentity.TaskID, SessionID: "session-new"}
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

	err := service.TransferSessionWithDurableAttachmentPreparation(
		ctx, sourceIdentity.TaskID, sourceIdentity.SessionID, destinationIdentity.SessionID, nil, nil,
	)
	if !errors.Is(err, ErrSessionIdentityMismatch) {
		t.Fatalf("durable transfer error = %v, want ErrSessionIdentityMismatch", err)
	}
	pending, err := repo.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Claim.Identity != sourceIdentity {
		t.Fatalf("pending claims after rejected durable transfer = %#v, want source identity", pending)
	}
}

func TestTransfersRejectReincarnatedIdentityAwareSendNowClaim(t *testing.T) {
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

	for _, transfer := range transfers {
		for _, accepted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/accepted=%t", transfer.name, accepted), func(t *testing.T) {
				ctx := context.Background()
				repo := newTestSQLiteRepo(t).(*sqliteRepository)
				originalSource := QueueSessionIdentity{
					TaskID: "task-1", SessionID: "session-old", SessionIncarnationID: "incarnation-old",
				}
				liveSource := originalSource
				liveSource.SessionIncarnationID = "incarnation-recreated"
				destination := QueueSessionIdentity{
					TaskID: originalSource.TaskID, SessionID: "session-new", SessionIncarnationID: "incarnation-new",
				}
				seedQueueSessionIdentity(t, repo, originalSource)
				seedQueueSessionIdentity(t, repo, destination)
				first := &QueuedMessage{
					ID: "first", TaskID: originalSource.TaskID, SessionID: originalSource.SessionID,
					Content: "first", QueuedBy: QueuedByUser,
				}
				second := &QueuedMessage{
					ID: "second", TaskID: originalSource.TaskID, SessionID: originalSource.SessionID,
					Content: "second", QueuedBy: QueuedByUser,
				}
				if err := repo.InsertForSession(ctx, originalSource, first, DefaultMaxPerSession); err != nil {
					t.Fatal(err)
				}
				if err := repo.InsertForSession(ctx, originalSource, second, DefaultMaxPerSession); err != nil {
					t.Fatal(err)
				}
				claim, err := repo.ClaimSendNowForSession(ctx, originalSource, []QueuedMessage{*first, *second})
				if err != nil {
					t.Fatal(err)
				}
				if accepted {
					if err := repo.MarkPendingSendNowClaimAccepted(ctx, claim); err != nil {
						t.Fatal(err)
					}
				}
				seedQueueSessionIdentity(t, repo, liveSource)

				err = transfer.run(ctx, repo, liveSource, destination)
				if !errors.Is(err, ErrSessionIdentityMismatch) {
					t.Fatalf("transfer error = %v, want ErrSessionIdentityMismatch", err)
				}
				pending, err := repo.ListPendingSendNowClaims(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(pending) != 1 {
					t.Fatalf("pending claims = %#v, want one preserved claim", pending)
				}
				preserved := pending[0]
				if preserved.Claim.ClaimID != claim.ClaimID || preserved.Claim.Identity != originalSource ||
					preserved.Accepted != accepted || preserved.Claim.Dispatch.Content != "first\n\nsecond" {
					t.Fatalf("preserved claim = %#v", preserved)
				}
				contents := []string{preserved.Claim.Sources[0].Content, preserved.Claim.Sources[1].Content}
				if contents[0] != "first" || contents[1] != "second" {
					t.Fatalf("preserved claim sources = %#v", preserved.Claim.Sources)
				}
				destinationEntries, err := repo.ListBySession(ctx, destination.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				if len(destinationEntries) != 0 {
					t.Fatalf("destination entries after rejected transfer = %#v", destinationEntries)
				}
			})
		}
	}
}
