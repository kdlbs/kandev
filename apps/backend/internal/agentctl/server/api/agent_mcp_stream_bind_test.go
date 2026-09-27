package api

import (
	"context"
	"errors"
	"testing"
	"time"

	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestWriteAgentStreamMCPRequestBindsBeforeWrite covers the "Sent and not
// sent" contract (system design part 2): a bind failure (the stream was
// already marked failed) must never reach the socket write, and the call
// completes as not-sent so the send loop retries it.
func TestWriteAgentStreamMCPRequestBindsBeforeWrite(t *testing.T) {
	log := newTestLogger()
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	server := &Server{mcpBackendClient: backend, logger: log}

	errCh := make(chan error, 1)
	go func() {
		errCh <- backend.RequestPayload(context.Background(), "mcp.tools.call", nil, nil)
	}()
	first := <-backend.GetRequestChannel()

	// Mark the stream this request would bind to as already failed (as a
	// disconnect or supersede would), then attempt to deliver it there.
	backend.FailStreamRequests("dead-stream", errors.New("agent stream disconnected"))

	writeCalled := false
	ok := server.writeAgentStreamMCPRequest(first, "dead-stream", func([]byte) error {
		writeCalled = true
		return nil
	})
	if !ok {
		t.Fatal("writeAgentStreamMCPRequest returned false on a bind failure, want true (keep the stream open)")
	}
	if writeCalled {
		t.Fatal("writeAgentStreamMCPRequest wrote to the socket after a bind failure")
	}

	// A bind failure is "not sent": the send loop retries with the same
	// logical call instead of failing it.
	var second *ws.Message
	select {
	case second = <-backend.GetRequestChannel():
	case <-time.After(time.Second):
		t.Fatal("request was not retried after a bind failure")
	}
	if second.ID != first.ID {
		t.Fatalf("retried request id = %q, want %q", second.ID, first.ID)
	}

	response, err := ws.NewResponse(second.ID, second.Action, nil)
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	if ok := server.writeAgentStreamMCPRequest(second, "live-stream", func([]byte) error { return nil }); !ok {
		t.Fatal("writeAgentStreamMCPRequest returned false on a successful bind and write")
	}
	backend.HandleResponse(response)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("request error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not complete after a successful retry")
	}
}

// TestWriteAgentStreamMCPRequestWriteFailureReturnsUnknownOutcome covers the
// other half of "Sent and not sent": a write failure happens after a
// successful bind, so the call is treated as sent-but-unconfirmed and fails
// with ErrKandevCallOutcomeUnknown rather than being retried.
func TestWriteAgentStreamMCPRequestWriteFailureReturnsUnknownOutcome(t *testing.T) {
	log := newTestLogger()
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	server := &Server{mcpBackendClient: backend, logger: log}

	errCh := make(chan error, 1)
	go func() {
		errCh <- backend.RequestPayload(context.Background(), "mcp.tools.call", nil, nil)
	}()
	request := <-backend.GetRequestChannel()

	ok := server.writeAgentStreamMCPRequest(request, "live-stream", func([]byte) error {
		return errors.New("socket write failed")
	})
	if ok {
		t.Fatal("writeAgentStreamMCPRequest returned true on a write failure, want false (stop the stream)")
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, mcpserver.ErrKandevCallOutcomeUnknown) {
			t.Fatalf("request error = %v, want %v", err, mcpserver.ErrKandevCallOutcomeUnknown)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not fail after a write failure")
	}
}
