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

// testAttachment is a controllable AttachmentSnapshot source for exercising
// RequestPayload's send loop (system design part 2 "Send loop") without a
// real agentctl process manager.
type testAttachment struct {
	mu                sync.Mutex
	attached          bool
	episode           uint64
	attachedCh        chan struct{}
	budgetCh          chan struct{}
	closeAttachedOnce sync.Once
	closeBudgetOnce   sync.Once
}

func newTestAttachment(attached bool) *testAttachment {
	return &testAttachment{
		attached:   attached,
		episode:    1,
		attachedCh: make(chan struct{}),
		budgetCh:   make(chan struct{}),
	}
}

func (a *testAttachment) snapshot() AttachmentSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AttachmentSnapshot{
		Attached:        a.attached,
		Episode:         a.episode,
		AttachedCh:      a.attachedCh,
		BudgetExhausted: a.budgetCh,
	}
}

// detach flips the snapshot to detached without ending the episode.
func (a *testAttachment) detach() {
	a.mu.Lock()
	a.attached = false
	a.mu.Unlock()
}

// reattach ends the current detached episode: flips Attached and closes
// AttachedCh, exactly like a real confirm would (system design part 2 "Send
// loop" step 3).
func (a *testAttachment) reattach() {
	a.mu.Lock()
	a.attached = true
	a.mu.Unlock()
	a.closeAttachedOnce.Do(func() { close(a.attachedCh) })
}

func (a *testAttachment) exhaustBudget() {
	a.closeBudgetOnce.Do(func() { close(a.budgetCh) })
}

// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.1
func TestRequestPayloadWaitsWhileDetachedThenSendsOnReattach(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	att := newTestAttachment(false)
	client.SetAttachmentSnapshotter(att.snapshot)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RequestPayload(context.Background(), "test.action", nil, nil)
	}()

	select {
	case <-client.GetRequestChannel():
		t.Fatal("request was sent while detached")
	case <-time.After(50 * time.Millisecond):
	}

	att.reattach()

	var request *ws.Message
	select {
	case request = <-client.GetRequestChannel():
	case <-time.After(time.Second):
		t.Fatal("request was not sent after reattach")
	}

	response, err := ws.NewResponse(request.ID, request.Action, nil)
	require.NoError(t, err)
	client.HandleResponse(response)
	require.NoError(t, <-errCh)
}

// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.2
func TestRequestPayloadDetachedBudgetExhaustedReturnsGuidance(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	att := newTestAttachment(false)
	client.SetAttachmentSnapshotter(att.snapshot)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RequestPayload(context.Background(), "test.action", nil, nil)
	}()

	att.exhaustBudget()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, ErrOfflineBudgetExhausted)
	case <-time.After(time.Second):
		t.Fatal("request did not fail on budget exhaustion")
	}

	select {
	case <-client.GetRequestChannel():
		t.Fatal("request was sent after budget exhaustion")
	default:
	}
}

func TestRequestPayloadDetachedWaitRespectsContextCancellation(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	att := newTestAttachment(false)
	client.SetAttachmentSnapshotter(att.snapshot)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RequestPayload(ctx, "test.action", nil, nil)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("request did not observe context cancellation while detached")
	}
}

func TestRequestPayloadSendTimeoutWhileAttachedReturnsError(t *testing.T) {
	original := requestSendTimeout
	requestSendTimeout = 20 * time.Millisecond
	t.Cleanup(func() { requestSendTimeout = original })

	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)

	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.EqualError(t, err, "timeout sending request to agent stream")
}

// TestRequestPayloadSendTimeoutWhileNowDetachedRetries covers the send
// loop's step-2-to-step-1 transition: the attached send attempt times out,
// but by the time it re-checks attachment it is detached, so it retries
// into the detached wait instead of failing (system design part 2 "Send
// loop" step 2).
func TestRequestPayloadSendTimeoutWhileNowDetachedRetries(t *testing.T) {
	original := requestSendTimeout
	requestSendTimeout = 30 * time.Millisecond
	t.Cleanup(func() { requestSendTimeout = original })

	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	att := newTestAttachment(true)
	client.SetAttachmentSnapshotter(att.snapshot)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RequestPayload(context.Background(), "test.action", nil, nil)
	}()

	// No consumer yet: the attached send attempt will time out. Detach
	// shortly after so its re-check sees "now detached" well before the
	// timer fires, then give the timer time to fire and land in the
	// detached wait. Never read GetRequestChannel during this window: a
	// read here would itself pair with the still-open attached send case
	// and mask the retry behavior under test.
	time.Sleep(5 * time.Millisecond)
	att.detach()
	time.Sleep(150 * time.Millisecond)

	select {
	case err := <-errCh:
		t.Fatalf("request failed instead of retrying into the detached wait: %v", err)
	default:
	}

	att.reattach()

	var request *ws.Message
	select {
	case request = <-client.GetRequestChannel():
	case <-time.After(time.Second):
		t.Fatal("request was not retried after reattach")
	}
	response, err := ws.NewResponse(request.ID, request.Action, nil)
	require.NoError(t, err)
	client.HandleResponse(response)
	require.NoError(t, <-errCh)
}

// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.2
func TestBindRequestToStreamAfterStreamFailedReturnsError(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)

	client.FailStreamRequests("dead-stream", errors.New("stream ended"))

	err := client.BindRequestToStream("some-request-id", "dead-stream")
	require.Error(t, err)
}

// TestFailRequestNotSentRetriesSendLoop exercises the full "Sent and not
// sent" step 3 contract: a call the writer never wrote (surfaced through
// FailRequestNotSent, the method writeAgentStreamMCPRequest calls on a bind
// failure) is retried from the top of the send loop rather than failed.
func TestFailRequestNotSentRetriesSendLoop(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RequestPayload(context.Background(), "test.action", nil, nil)
	}()

	first := <-client.GetRequestChannel()
	client.FailRequestNotSent(first.ID)

	second := <-client.GetRequestChannel()
	require.Equal(t, first.ID, second.ID, "retried request must be the same logical call")

	response, err := ws.NewResponse(second.ID, second.Action, nil)
	require.NoError(t, err)
	client.HandleResponse(response)
	require.NoError(t, <-errCh)
}
