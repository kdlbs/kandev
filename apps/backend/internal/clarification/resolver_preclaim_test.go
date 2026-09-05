package clarification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type preClaimBlockingMessageStore struct {
	stubMessageStore
	entered chan struct{}
}

func (s *preClaimBlockingMessageStore) FindMessagesByPendingID(
	ctx context.Context,
	pendingID string,
) ([]*taskmodels.Message, error) {
	_, s.findHasDeadline = ctx.Deadline()
	close(s.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

type preClaimBlockingMessageCreator struct {
	stubMessageCreator
	claimCalls    int
	claimDeadline bool
	entered       chan struct{}
}

func (s *preClaimBlockingMessageCreator) CompleteActiveClarificationBundle(
	ctx context.Context,
	_ string,
	_ string,
	_ map[string]interface{},
) ([]*taskmodels.Message, bool, error) {
	s.claimCalls++
	_, s.claimDeadline = ctx.Deadline()
	close(s.entered)
	<-ctx.Done()
	return nil, false, ctx.Err()
}

func shortenPreClaimTimeout(t *testing.T) {
	t.Helper()
	previous := clarificationPreClaimTimeout
	clarificationPreClaimTimeout = time.Millisecond
	t.Cleanup(func() { clarificationPreClaimTimeout = previous })
}

func TestResolverPreClaimIdentityUsesBoundedContextAndReturnsRetryableError(t *testing.T) {
	shortenPreClaimTimeout(t)
	repo := &preClaimBlockingMessageStore{
		stubMessageStore: stubMessageStore{},
		entered:          make(chan struct{}),
	}
	resolver := NewResolver(
		NewStore(time.Minute),
		repo,
		nil,
		&stubAuthorizer{},
		nil,
		nil,
		nil,
		logger.Default(),
	)

	_, claimed, err := resolver.ResolveBundle(context.Background(), "pending-preclaim-identity", Outcome{})
	if claimed {
		t.Fatal("identity timeout reported a claimed response")
	}
	if err == nil || !strings.Contains(err.Error(), "temporarily unavailable") {
		t.Fatalf("identity timeout error = %v, want retryable temporary-unavailable classification", err)
	}
	if !repo.findHasDeadline {
		t.Fatal("identity lookup received no pre-claim deadline")
	}
}

func TestResolverPreClaimClaimFailureDoesNotDeliver(t *testing.T) {
	shortenPreClaimTimeout(t)
	const pendingID = "pending-preclaim-claim"
	repo := &stubMessageStore{messages: map[string][]*taskmodels.Message{
		pendingID: {resolverDeliveryMessage(pendingID, "message-preclaim-claim", "turn-1")},
	}}
	creator := &preClaimBlockingMessageCreator{entered: make(chan struct{})}
	resolver := NewResolver(
		NewStore(time.Minute),
		repo,
		creator,
		&stubAuthorizer{},
		nil,
		nil,
		nil,
		logger.Default(),
	)

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, Outcome{
		Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"yes"}}},
	})
	if claimed {
		t.Fatal("claim timeout reported a claimed response")
	}
	if err == nil || !strings.Contains(err.Error(), "temporarily unavailable") {
		t.Fatalf("claim timeout error = %v, want retryable temporary-unavailable classification", err)
	}
	if creator.claimCalls != 1 || !creator.claimDeadline {
		t.Fatalf("claim calls=%d deadline=%v, want one call with a deadline", creator.claimCalls, creator.claimDeadline)
	}
	if got := stringFromMetadata(repo.messages[pendingID][0].Metadata, metaStatusKey); got != string(StatusPending) {
		t.Fatalf("status after pre-claim timeout = %q, want pending", got)
	}
}

func TestResolverCallerCancellationIsNotClassifiedAsPreClaimTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &stubMessageStore{findErr: context.Canceled}
	resolver := NewResolver(
		NewStore(time.Minute),
		repo,
		nil,
		&stubAuthorizer{},
		nil,
		nil,
		nil,
		logger.Default(),
	)

	_, claimed, err := resolver.ResolveBundle(ctx, "pending-caller-cancelled", Outcome{})
	if claimed {
		t.Fatal("caller cancellation reported a claimed response")
	}
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation error = %v, want context.Canceled", err)
	}
	if IsPreClaimTimeoutError(err) {
		t.Fatalf("caller cancellation was classified as internal timeout: %v", err)
	}
}
