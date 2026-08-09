package client

import (
	"context"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestProbeBackgroundWorkloads_SuccessMapping verifies AC-45's success
// clause: a well-formed live/settled/unknown response passes through
// unchanged, and the wire request carries the ACP session id verbatim.
func TestProbeBackgroundWorkloads_SuccessMapping(t *testing.T) {
	for _, want := range []string{"live", "settled", "unknown"} {
		t.Run(want, func(t *testing.T) {
			var gotAction string
			var gotPayload probeBackgroundWorkloadsRequest
			c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
				gotAction = msg.Action
				_ = msg.ParsePayload(&gotPayload)
				resp, _ := ws.NewResponse(msg.ID, msg.Action, probeBackgroundWorkloadsResponse{Result: want})
				return resp
			})
			defer ts.Close()
			defer c.Close()

			got, err := c.ProbeBackgroundWorkloads(context.Background(), "acp-session-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != want {
				t.Fatalf("result = %q, want %q", got, want)
			}
			if gotAction != "agent.background.probe" {
				t.Fatalf("action = %q, want agent.background.probe", gotAction)
			}
			if gotPayload.SessionID != "acp-session-1" {
				t.Fatalf("wire session_id = %q, want acp-session-1 (the ACP id, verbatim)", gotPayload.SessionID)
			}
		})
	}
}

// TestProbeBackgroundWorkloads_UnrecognisedResultMapsToUnknown verifies that
// a result outside the three literals resolves to unknown with a nil error —
// AC-46's "response carries a result outside the three literals" condition.
func TestProbeBackgroundWorkloads_UnrecognisedResultMapsToUnknown(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewResponse(msg.ID, msg.Action, probeBackgroundWorkloadsResponse{Result: "bogus"})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	got, err := c.ProbeBackgroundWorkloads(context.Background(), "acp-session-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "unknown" {
		t.Fatalf("result = %q, want unknown", got)
	}
}

// TestProbeBackgroundWorkloads_TransportErrorPropagates verifies that a
// transport-level failure (here: a timeout) returns a non-nil error so the
// caller's error-check-first discipline has something to check — the port's
// own "map every error to unknown" rule lives one layer up, in
// lifecycle.Manager.ProbeBackgroundWorkloads, not in this Client method.
func TestProbeBackgroundWorkloads_TransportErrorPropagates(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		return nil // never respond
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	got, err := c.ProbeBackgroundWorkloads(ctx, "acp-session-3")
	if err == nil {
		t.Fatal("expected a non-nil error on timeout")
	}
	if got != "unknown" {
		t.Fatalf("result on error = %q, want unknown (the caller must not read it, but it should still be the safe value)", got)
	}
}

// TestProbeBackgroundWorkloads_CancelledContext verifies AC-46's tenth
// condition at the Client layer: an already-done context resolves without a
// frame ever leaving sendStreamRequest.
func TestProbeBackgroundWorkloads_CancelledContext(t *testing.T) {
	called := false
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		called = true
		resp, _ := ws.NewResponse(msg.ID, msg.Action, probeBackgroundWorkloadsResponse{Result: "live"})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := c.ProbeBackgroundWorkloads(ctx, "acp-session-4")
	if err == nil {
		t.Fatal("expected a non-nil error for an already-cancelled context")
	}
	if got != "unknown" {
		t.Fatalf("result = %q, want unknown", got)
	}
	// Give any stray goroutine a moment, then confirm no frame reached the server.
	time.Sleep(20 * time.Millisecond)
	if called {
		t.Fatal("expected no request to reach the server for an already-cancelled context")
	}
}
