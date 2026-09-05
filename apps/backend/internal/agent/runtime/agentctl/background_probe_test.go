package client

import (
	"context"
	"strings"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// AC-45: request carries session_id and nothing else; a "live" response
// round-trips into ProbeResultLive.
func TestProbeBackgroundWorkloads_Success(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.background.probe" {
			t.Errorf("expected action 'agent.background.probe', got %q", msg.Action)
		}
		var payload backgroundProbeRequest
		if err := msg.ParsePayload(&payload); err != nil {
			t.Errorf("failed to parse payload: %v", err)
		}
		if payload.SessionID != "sess-1" {
			t.Errorf("expected session_id 'sess-1', got %q", payload.SessionID)
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"result": "live"})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := c.ProbeBackgroundWorkloads(ctx, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != ProbeResultLive {
		t.Errorf("expected %q, got %q", ProbeResultLive, result)
	}
}

func TestProbeBackgroundWorkloads_SettledAndUnknownRoundTrip(t *testing.T) {
	for _, want := range []ProbeResult{ProbeResultSettled, ProbeResultUnknown} {
		t.Run(string(want), func(t *testing.T) {
			c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
				resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"result": string(want)})
				return resp
			})
			defer ts.Close()
			defer c.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := c.ProbeBackgroundWorkloads(ctx, "sess-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != want {
				t.Errorf("expected %q, got %q", want, result)
			}
		})
	}
}

// AC-46: stream disconnection resolves to unknown.
func TestProbeBackgroundWorkloads_NotConnected_Unknown(t *testing.T) {
	log := newTestLogger()
	c := &Client{
		baseURL:         "http://localhost:0",
		logger:          log,
		pendingRequests: make(map[string]chan *ws.Message),
	}

	result, err := c.ProbeBackgroundWorkloads(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error when stream not connected")
	}
	if !strings.Contains(err.Error(), "agent stream not connected") {
		t.Fatalf("expected 'agent stream not connected' error, got: %v", err)
	}
	if result != ProbeResultUnknown {
		t.Errorf("expected %q, got %q", ProbeResultUnknown, result)
	}
}

// AC-46: context deadline exceeded resolves to unknown.
func TestProbeBackgroundWorkloads_ContextDeadlineExceeded_Unknown(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		// Never responds — the caller's context expires while waiting.
		return nil
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := c.ProbeBackgroundWorkloads(ctx, "sess-1")
	if err == nil {
		t.Fatal("expected an error on context deadline")
	}
	if result != ProbeResultUnknown {
		t.Errorf("expected %q, got %q", ProbeResultUnknown, result)
	}
}

// AC-46: a WS error frame (e.g. ErrorCodeUnknownAction, from an older
// agentctl binary that predates this action) resolves to unknown.
func TestProbeBackgroundWorkloads_ErrorFrame_Unknown(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, "unknown action: agent.background.probe", nil)
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := c.ProbeBackgroundWorkloads(ctx, "sess-1")
	if err == nil {
		t.Fatal("expected an error for a WS error frame")
	}
	if result != ProbeResultUnknown {
		t.Errorf("expected %q, got %q", ProbeResultUnknown, result)
	}
}

// AC-46: an absent/unparseable body resolves to unknown.
func TestProbeBackgroundWorkloads_UnparseableBody_Unknown(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"result": 42})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := c.ProbeBackgroundWorkloads(ctx, "sess-1")
	if err == nil {
		t.Fatal("expected an error for an unparseable body")
	}
	if result != ProbeResultUnknown {
		t.Errorf("expected %q, got %q", ProbeResultUnknown, result)
	}
}

// AC-46: a result value outside the three literals resolves to unknown.
func TestProbeBackgroundWorkloads_UnrecognizedResultLiteral_Unknown(t *testing.T) {
	c, ts := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"result": "definitely-not-a-real-result"})
		return resp
	})
	defer ts.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := c.ProbeBackgroundWorkloads(ctx, "sess-1")
	if err == nil {
		t.Fatal("expected an error for an unrecognized result literal")
	}
	if result != ProbeResultUnknown {
		t.Errorf("expected %q, got %q", ProbeResultUnknown, result)
	}
}
