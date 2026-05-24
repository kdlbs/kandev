package websocket

import (
	"context"
	"testing"
)

func TestDispatcher_RegisterAndDispatch(t *testing.T) {
	d := NewDispatcher()

	d.RegisterFunc("ping", func(_ context.Context, msg *Message) (*Message, error) {
		return &Message{ID: msg.ID, Action: "pong"}, nil
	})

	if !d.HasHandler("ping") {
		t.Fatal("expected HasHandler(\"ping\") to be true")
	}

	resp, err := d.Dispatch(context.Background(), &Message{ID: "1", Action: "ping"})
	if err != nil {
		t.Fatalf("dispatch returned err: %v", err)
	}
	if resp == nil || resp.Action != "pong" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestDispatcher_UnknownActionReturnsError(t *testing.T) {
	d := NewDispatcher()

	resp, err := d.Dispatch(context.Background(), &Message{ID: "1", Action: "nope"})
	if err != nil {
		t.Fatalf("dispatch returned err: %v", err)
	}
	if resp == nil {
		t.Fatal("expected error response, got nil")
	}
	if resp.Type != MessageTypeError {
		t.Errorf("expected error message type, got %q", resp.Type)
	}

	var payload ErrorPayload
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse error payload: %v", err)
	}
	if payload.Code != ErrorCodeUnknownAction {
		t.Errorf("expected error code %q, got %q",
			ErrorCodeUnknownAction, payload.Code)
	}
}
