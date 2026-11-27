package clarification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryWaiterKeepsConfirmedEntryAfterMapRemoval(t *testing.T) {
	store := NewStore(time.Minute)
	req := &Request{PendingID: "retry-pin", SessionID: "session", Questions: []Question{{Prompt: "Continue?"}}}
	store.CreateRetryRequest(req)
	_, created, missed, wait := store.CreateRetryRequestWithWaiter(req)
	require.False(t, created)
	require.False(t, missed)
	response := &Response{Answers: []Answer{{QuestionID: "q1", CustomText: "yes"}}}
	entered := make(chan struct{})
	store.SetOnWaitEntered(func(string) { close(entered) })
	original := make(chan error, 1)
	go func() { _, err := store.WaitForResponse(context.Background(), req.PendingID); original <- err }()
	<-entered
	store.SetOnWaitEntered(nil)
	confirmations := 0
	require.NoError(t, store.RespondWithDeliveryConfirmation(context.Background(), req.PendingID, response, func() error { confirmations++; return nil }))
	require.NoError(t, <-original)
	replacement := &Request{PendingID: req.PendingID, SessionID: "replacement", Questions: []Question{{Prompt: "Replacement?"}}}
	_, isNew := store.CreateRequest(replacement)
	require.True(t, isNew)
	got, err := wait(context.Background(), nil)
	require.NoError(t, err)
	require.Same(t, response, got)
	require.Equal(t, 1, confirmations)
	current, exists := store.GetRequest(req.PendingID)
	require.True(t, exists)
	require.Same(t, replacement, current)
	store.CancelRequest(req.PendingID)
}

func TestRetryWaiterRechecksDetachedOwnershipBeforeRecordedReplay(t *testing.T) {
	store := NewStore(time.Minute)
	req := &Request{PendingID: "retry-missed", SessionID: "session", Questions: []Question{{Prompt: "Continue?"}}}
	_, _, missed, wait := store.CreateRetryRequestWithWaiter(req)
	require.False(t, missed)
	store.CancelRequest(req.PendingID)
	err := store.RespondWithDeliveryConfirmation(context.Background(), req.PendingID, &Response{}, func() error { t.Fatal("detached response invoked live confirmation"); return nil })
	require.ErrorIs(t, err, ErrNotFound)
	got, err := wait(context.Background(), &Response{PendingID: req.PendingID})
	require.True(t, errors.Is(err, ErrNotFound))
	require.Nil(t, got)
}

func TestRetryWaiterJoinsConfirmationClaimedDuringSessionCancellation(t *testing.T) {
	store := NewStore(time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := &Request{PendingID: "retry-cancel-claim", SessionID: "session"}
	store.CreateRetryRequest(req)
	entered := make(chan struct{})
	store.SetOnWaitEntered(func(string) { close(entered) })
	original := make(chan error, 1)
	go func() { _, err := store.WaitForResponse(ctx, req.PendingID); original <- err }()
	<-entered
	store.SetOnWaitEntered(nil)
	loaded, respondRelease := make(chan struct{}), make(chan struct{})
	store.SetOnRespondLoaded(func(string) { close(loaded); <-respondRelease })
	confirming, finish := make(chan struct{}), make(chan struct{})
	responding := make(chan error, 1)
	go func() {
		responding <- store.RespondWithDeliveryConfirmation(ctx, req.PendingID, &Response{}, func() error { close(confirming); <-finish; return nil })
	}()
	<-loaded
	parked, release := make(chan struct{}), make(chan struct{})
	store.onCancelSessionEntered = func(string) { close(parked); <-release }
	cancelled := make(chan []string, 1)
	go func() { cancelled <- store.CancelSession(req.SessionID) }()
	<-parked
	close(respondRelease)
	<-confirming
	close(release)
	ids := <-cancelled
	_, created, missed, wait := store.CreateRetryRequestWithWaiter(req)
	close(finish)
	require.NoError(t, <-original)
	require.NoError(t, <-responding)
	require.Empty(t, ids, "session cancellation must retain a claimed confirmation")
	require.False(t, created, "retry must join the entry claimed during cancellation")
	require.False(t, missed)
	_, err := wait(ctx, nil)
	require.NoError(t, err)
}
