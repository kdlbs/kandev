package mcp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// fakeAttachment is a controllable AttachmentSnapshot source.
type fakeAttachment struct {
	mu   sync.Mutex
	snap AttachmentSnapshot
	ch   chan struct{}
}

func newFakeAttachment() *fakeAttachment {
	f := &fakeAttachment{ch: make(chan struct{})}
	f.snap.ChangeCh = f.ch
	return f
}

func (f *fakeAttachment) Snapshot() AttachmentSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap
}

func (f *fakeAttachment) set(mutate func(*AttachmentSnapshot)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mutate(&f.snap)
	close(f.ch)
	f.ch = make(chan struct{})
	f.snap.ChangeCh = f.ch
}

func newWaitingClient(t *testing.T) (*ChannelBackendClient, *fakeAttachment) {
	t.Helper()
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	att := newFakeAttachment()
	client.SetAttachmentSnapshotter(att.Snapshot)
	return client, att
}

func requestAsync(client *ChannelBackendClient, ctx context.Context) chan error {
	errCh := make(chan error, 1)
	go func() { errCh <- client.RequestPayload(ctx, "test.action", nil, nil) }()
	return errCh
}

func TestRequestWaitsWhileDetachedThenDelivers(t *testing.T) {
	client, att := newWaitingClient(t)
	errCh := requestAsync(client, context.Background())

	select {
	case <-client.GetRequestChannel():
		t.Fatal("request published while detached")
	case err := <-errCh:
		t.Fatalf("call returned while detached: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	request := <-client.GetRequestChannel()
	require.NoError(t, client.BindRequestToStream(request.ID, "s1"))
	response, err := ws.NewResponse(request.ID, request.Action, nil)
	require.NoError(t, err)
	client.HandleResponse(response)
	require.NoError(t, <-errCh)
}

func TestRequestWaitsWhileAttachedButNotReady(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Attached = true })
	errCh := requestAsync(client, context.Background())
	select {
	case err := <-errCh:
		t.Fatalf("call returned while not writer-ready: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestWaitingRequestIsWithdrawnOnContextCancel(t *testing.T) {
	client, _ := newWaitingClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := requestAsync(client, ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	require.ErrorIs(t, <-errCh, context.Canceled)
	client.pendingMu.Lock()
	defer client.pendingMu.Unlock()
	require.Empty(t, client.pending)
}

func TestExhaustedAndDetachedFailsImmediately(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Exhausted = true })
	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.ErrorIs(t, err, ErrOfflineBudgetExhausted)
}

func TestExhaustedAttachedReadyStillWrites(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Exhausted, s.Attached, s.Ready, s.Epoch = true, true, true, 3 })
	errCh := requestAsync(client, context.Background())
	request := <-client.GetRequestChannel()
	require.NoError(t, client.BindRequestToStream(request.ID, "s1"))
	response, err := ws.NewResponse(request.ID, request.Action, nil)
	require.NoError(t, err)
	client.HandleResponse(response)
	require.NoError(t, <-errCh)
}

func TestExhaustedAttachedNotReadyWaits(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Exhausted, s.Attached = true, true })
	errCh := requestAsync(client, context.Background())
	select {
	case err := <-errCh:
		t.Fatalf("call returned: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestBudgetFailurePassFailsWaitingCalls(t *testing.T) {
	client, att := newWaitingClient(t)
	errCh := requestAsync(client, context.Background())
	time.Sleep(50 * time.Millisecond)
	att.set(func(s *AttachmentSnapshot) { s.Exhausted = true })
	require.ErrorIs(t, <-errCh, ErrOfflineBudgetExhausted)
}

func TestAttachedReadySendTimeoutFailsAfterSendWindow(t *testing.T) {
	client, att := newWaitingClient(t)
	client.sendTimeout = 80 * time.Millisecond
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.EqualError(t, err, "timeout sending request to agent stream")
}

func TestSendTimeoutRestartsAtNextWriterReady(t *testing.T) {
	client, att := newWaitingClient(t)
	client.sendTimeout = 300 * time.Millisecond
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	errCh := requestAsync(client, context.Background())
	time.Sleep(200 * time.Millisecond)
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready = true, false })
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 2 })
	time.Sleep(200 * time.Millisecond) // 400ms since the first attempt, 200ms since the restart
	select {
	case err := <-errCh:
		t.Fatalf("timer was not restarted at the next writer-ready: %v", err)
	default:
	}
	require.Error(t, <-errCh)
}

func TestSameEpochChangeKeepsSendTimer(t *testing.T) {
	client, att := newWaitingClient(t)
	client.sendTimeout = 250 * time.Millisecond
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	start := time.Now()
	errCh := requestAsync(client, context.Background())
	time.Sleep(100 * time.Millisecond)
	att.set(func(s *AttachmentSnapshot) { s.Exhausted = true }) // same epoch, still ready
	require.EqualError(t, <-errCh, "timeout sending request to agent stream")
	require.Less(t, time.Since(start), 450*time.Millisecond)
}

func TestNoWaitCallKeepsSendTimeoutWhileDetached(t *testing.T) {
	client, _ := newWaitingClient(t)
	client.sendTimeout = 80 * time.Millisecond
	err := client.RequestPayload(WithNoWait(context.Background()), "test.action", nil, nil)
	require.EqualError(t, err, "timeout sending request to agent stream")
}

func TestClientWithoutSnapshotterKeepsFiveSecondSend(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	client.sendTimeout = 80 * time.Millisecond
	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.EqualError(t, err, "timeout sending request to agent stream")
}

func TestStreamEndFailsBoundCallWithOutcomeUnknown(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	errCh := requestAsync(client, context.Background())
	request := <-client.GetRequestChannel()
	require.NoError(t, client.BindRequestToStream(request.ID, "s1"))
	client.FailStreamRequests("s1", ErrKandevCallOutcomeUnknown)
	err := <-errCh
	require.ErrorIs(t, err, ErrKandevCallOutcomeUnknown)
	require.Contains(t, err.Error(), "outcome unknown")
}

func TestUnboundCallIsNotFailedByStreamEnd(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	errCh := requestAsync(client, context.Background())
	<-client.GetRequestChannel() // writer received it but did not bind
	client.FailStreamRequests("s1", ErrKandevCallOutcomeUnknown)
	select {
	case err := <-errCh:
		t.Fatalf("unbound call failed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestFailRequestNotSentReturnsCallToWaiting(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	errCh := requestAsync(client, context.Background())
	first := <-client.GetRequestChannel()
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready = false, false })
	client.FailRequestNotSent(first.ID)

	select {
	case <-client.GetRequestChannel():
		t.Fatal("not-sent call republished while detached")
	case err := <-errCh:
		t.Fatalf("not-sent call returned: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 2 })
	second := <-client.GetRequestChannel()
	require.Equal(t, first.ID, second.ID)
	require.NoError(t, client.BindRequestToStream(second.ID, "s2"))
	response, err := ws.NewResponse(second.ID, second.Action, nil)
	require.NoError(t, err)
	client.HandleResponse(response)
	require.NoError(t, <-errCh)
}

func TestBindRejectsWithdrawnCall(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	ctx, cancel := context.WithCancel(context.Background())
	errCh := requestAsync(client, ctx)
	request := <-client.GetRequestChannel()
	cancel()
	require.ErrorIs(t, <-errCh, context.Canceled)
	require.True(t, errors.Is(client.BindRequestToStream(request.ID, "s1"), ErrRequestWithdrawn))
}

func TestResponseThenStreamEndKeepsResponse(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	errCh := requestAsync(client, context.Background())
	request := <-client.GetRequestChannel()
	require.NoError(t, client.BindRequestToStream(request.ID, "s1"))
	response, err := ws.NewResponse(request.ID, request.Action, nil)
	require.NoError(t, err)
	client.HandleResponse(response)
	client.FailStreamRequests("s1", ErrKandevCallOutcomeUnknown)
	require.NoError(t, <-errCh)
}

func TestFailEncodeClaimsOnlyIfPending(t *testing.T) {
	client, att := newWaitingClient(t)
	att.set(func(s *AttachmentSnapshot) { s.Attached, s.Ready, s.Epoch = true, true, 1 })
	errCh := requestAsync(client, context.Background())
	request := <-client.GetRequestChannel()
	client.FailRequest(request.ID, errors.New("encode failed"))
	require.EqualError(t, <-errCh, "encode failed")
	client.FailRequest(request.ID, errors.New("again")) // already gone: no panic, no block
}
