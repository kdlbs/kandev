package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type retryClaimBarrier struct {
	*svcMessageUpdater
	committed   chan struct{}
	release     chan struct{}
	finalizeErr error
}

func (b *retryClaimBarrier) CompleteActiveClarificationBundle(ctx context.Context, pendingID, status string, responses map[string]interface{}) ([]*models.Message, bool, error) {
	messages, claimed, err := b.svcMessageUpdater.CompleteActiveClarificationBundle(ctx, pendingID, status, responses)
	if claimed && err == nil {
		close(b.committed)
		<-b.release
	}
	return messages, claimed, err
}

func (b *retryClaimBarrier) FinalizeClarificationResponseDelivery(ctx context.Context, pendingID, status string, messages []*models.Message) ([]*models.Message, bool, error) {
	if b.finalizeErr != nil {
		return nil, false, b.finalizeErr
	}
	return b.svcMessageUpdater.FinalizeClarificationResponseDelivery(ctx, pendingID, status, messages)
}

func TestHandleAskUserQuestion_RetryDuringCommittedClaimWaitsForConfirmedDelivery(t *testing.T) {
	for _, test := range []struct {
		name        string
		rejected    bool
		finalizeErr error
	}{
		{name: "answer"},
		{name: "rejection", rejected: true},
		{name: "failed confirmation", finalizeErr: errors.New("confirmation unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) { runRetryCommittedClaim(t, test.rejected, test.finalizeErr) })
	}
}

func runRetryCommittedClaim(t *testing.T, rejected bool, finalizeErr error) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	taskID, sessionID, pendingID := seedRetrySession(t, ctx, svc, repo, "retry-committed-claim")
	seedRetryMessage(t, ctx, repo, taskID, sessionID, pendingID, "pending", nil)
	store := clarification.NewStore(time.Minute)
	t.Cleanup(func() { store.CancelSession(sessionID) })
	barrier := &retryClaimBarrier{
		svcMessageUpdater: &svcMessageUpdater{Service: svc},
		committed:         make(chan struct{}), release: make(chan struct{}), finalizeErr: finalizeErr,
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(barrier.release) }) }
	t.Cleanup(release)
	resumer := &countingDetachedResumer{}
	resolver := clarification.NewResolver(store, repo, barrier, svc, resumer, resumer, nil, testLogger(t))
	resolved := make(chan error, 1)
	outcome := clarification.Outcome{Rejected: rejected}
	if !rejected {
		outcome.Answers = []clarification.Answer{{QuestionID: "q1", SelectedOptions: []string{"opt-blue"}}}
	}
	go func() {
		_, _, err := resolver.ResolveBundle(ctx, pendingID, outcome)
		resolved <- err
	}()
	select {
	case <-barrier.committed:
	case <-ctx.Done():
		t.Fatal("resolver did not commit its provisional response")
	}
	waitEntered := make(chan struct{}, 1)
	store.SetOnWaitEntered(func(string) { waitEntered <- struct{}{} })
	h := NewHandlers(svc, nil, store, nil, &countingMessageCreator{}, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	done := make(chan askUserQuestionResult, 1)
	go func() {
		response, err := h.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, retryAskPayload(sessionID, taskID)))
		done <- askUserQuestionResult{response: response, err: err}
	}()
	var early *askUserQuestionResult
	select {
	case result := <-done:
		early = &result
	case <-waitEntered:
	case <-ctx.Done():
		t.Fatal("retry neither waited nor returned")
	}
	release()
	resolveErr := <-resolved
	result := early
	if result == nil {
		completed := <-done
		result = &completed
	}
	t.Logf("detached resumes = %d, resolver error = %v", resumer.calls.Load(), resolveErr)
	require.Nil(t, early, "a provisional durable claim must not return before delivery confirmation")
	require.Zero(t, resumer.calls.Load(), "the tool waiter and detached delivery must not both consume an answer")
	require.NoError(t, result.err)
	assertRetryConfirmedResult(t, repo, ctx, pendingID, result.response, rejected, finalizeErr, resolveErr)
}

func assertRetryConfirmedResult(t *testing.T, repo interface {
	FindMessagesByPendingID(context.Context, string) ([]*models.Message, error)
}, ctx context.Context, pendingID string, response *ws.Message, rejected bool, finalizeErr, resolveErr error) {
	t.Helper()
	messages, err := repo.FindMessagesByPendingID(ctx, pendingID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	if finalizeErr != nil {
		require.Error(t, resolveErr)
		assertWSError(t, response, ws.ErrorCodeInternalError)
		require.Equal(t, "pending", messages[0].Metadata["status"])
		return
	}
	require.NoError(t, resolveErr)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var body clarification.Response
	require.NoError(t, json.Unmarshal(response.Payload, &body))
	require.Equal(t, rejected, body.Rejected)
	require.NotContains(t, messages[0].Metadata, "response_delivery_pending")
}

func TestHandleAskUserQuestion_RetryDuringFinalizedLiveDeliveryWaitsForNotifier(t *testing.T) {
	for _, test := range []struct {
		name              string
		cancelBeforeRetry bool
		claimDuringRead   bool
	}{
		{name: "attached"},
		{name: "cancelled session", cancelBeforeRetry: true},
		{name: "claim during retry read", claimDuringRead: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runRetryFinalizedLiveDelivery(t, test.cancelBeforeRetry, test.claimDuringRead)
		})
	}
}

func runRetryFinalizedLiveDelivery(t *testing.T, cancelBeforeRetry, claimDuringRead bool) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	taskID, sessionID, pendingID := seedRetrySession(t, ctx, svc, repo, "retry-finalized-live")
	seedRetryMessage(t, ctx, repo, taskID, sessionID, pendingID, "pending", nil)
	store := clarification.NewStore(time.Minute)
	store.CreateRetryRequest(&clarification.Request{PendingID: pendingID, SessionID: sessionID, TaskID: taskID, Questions: retryQuestions(t)})
	entered := make(chan struct{}, 2)
	store.SetOnWaitEntered(func(string) { entered <- struct{}{} })
	original := make(chan error, 1)
	go func() { _, err := store.WaitForResponse(ctx, pendingID); original <- err }()
	<-entered
	notifying, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var notified atomic.Bool
	notifier := func(context.Context, clarification.PrimaryAnswered) {
		close(notifying)
		<-release
		notified.Store(true)
	}
	resumer := &countingDetachedResumer{}
	resolver := clarification.NewResolver(store, repo, &svcMessageUpdater{Service: svc}, svc, resumer, resumer, notifier, testLogger(t))
	resolved := make(chan error, 1)
	resolve := func() {
		_, _, err := resolver.ResolveBundle(ctx, pendingID, clarification.Outcome{Answers: []clarification.Answer{{QuestionID: "q1", SelectedOptions: []string{"opt-blue"}}}})
		resolved <- err
	}
	awaitNotifier := func() {
		select {
		case <-notifying:
		case <-ctx.Done():
			t.Error("live delivery did not reach the notifier")
		}
	}
	tracingRepo := &retryReadAnswerRepo{Repository: repo, onRead: func() {}}
	if claimDuringRead {
		tracingRepo.onRead = func() { go resolve(); awaitNotifier() }
	} else {
		go resolve()
		awaitNotifier()
	}
	if cancelBeforeRetry {
		store.CancelSession(sessionID)
	}
	h := NewHandlers(svc, nil, store, nil, &countingMessageCreator{}, tracingRepo, repo, nil, nil, nil, nil, nil, testLogger(t))
	done := make(chan askUserQuestionResult, 1)
	go func() {
		response, err := h.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, retryAskPayload(sessionID, taskID)))
		done <- askUserQuestionResult{response: response, err: err}
	}()
	var early *askUserQuestionResult
	select {
	case result := <-done:
		early = &result
	case <-entered:
	case <-ctx.Done():
		t.Fatal("retry neither waited nor returned")
	}
	unblock()
	require.NoError(t, <-resolved)
	require.NoError(t, <-original)
	if early == nil {
		result := <-done
		require.NoError(t, result.err)
		require.Equal(t, ws.MessageTypeResponse, result.response.Type)
	}
	require.True(t, notified.Load())
	require.Nil(t, early, "a retry must join live confirmation even after the delivery marker is cleared")
	require.Zero(t, resumer.calls.Load())
}
