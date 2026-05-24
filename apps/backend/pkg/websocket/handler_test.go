package websocket

import (
	"context"
	"sync"
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

// TestDispatcher_ConcurrentRegisterAndDispatch exercises the Dispatcher
// from many goroutines at once. The intent is that `go test -race` flags
// any data race on the underlying handlers map; without the RWMutex this
// test failed under -race with a "concurrent map read and map write"
// report.
func TestDispatcher_ConcurrentRegisterAndDispatch(t *testing.T) {
	d := NewDispatcher()

	const goroutines = 16
	const ops = 200

	noop := HandlerFunc(func(_ context.Context, msg *Message) (*Message, error) {
		return &Message{ID: msg.ID, Action: msg.Action}, nil
	})

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				// Alternate between Register and RegisterFunc so both
				// write paths are exercised under -race.
				if i%2 == 0 {
					d.Register("action", noop)
				} else {
					d.RegisterFunc("action", noop)
				}
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_, _ = d.Dispatch(context.Background(), &Message{
					ID:     "x",
					Action: "action",
				})
				_ = d.HasHandler("action")
			}
		}()
	}

	wg.Wait()
}
