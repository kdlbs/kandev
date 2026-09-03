package bus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestNATSCreateMsgHandler_MessageSubjectIsAuthoritative pins the receive-side
// half of the Subject contract: a wildcard NATS subscription resolves to the
// concrete subject only in msg.Subject, so the handler must overwrite whatever
// Subject the publisher marshaled (stale, forged, or absent) with it, while
// leaving the rest of the event as published.
//
// createMsgHandler is exercised directly — it never touches the connection, so
// this needs no NATS server.
func TestNATSCreateMsgHandler_MessageSubjectIsAuthoritative(t *testing.T) {
	published := &Event{
		ID:        "evt-1",
		Type:      "shell.output",
		Subject:   "shell.output.forged-session",
		Source:    "test",
		Timestamp: time.Unix(1700000000, 0).UTC(),
		Data:      map[string]interface{}{"chunk": "hello"},
	}
	data, err := json.Marshal(published)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	receivedCh := make(chan *Event, 1)
	b := &NATSEventBus{logger: newTestLogger(t)}
	handler := b.createMsgHandler("shell.output.*", func(_ context.Context, e *Event) error {
		receivedCh <- e
		return nil
	})

	handler(&nats.Msg{Subject: "shell.output.sess-1", Data: data})

	select {
	case got := <-receivedCh:
		if got.Subject != "shell.output.sess-1" {
			t.Errorf("Subject = %q, want the delivering msg.Subject shell.output.sess-1", got.Subject)
		}
		if got.EffectiveSubject() != "shell.output.sess-1" {
			t.Errorf("EffectiveSubject() = %q, want shell.output.sess-1", got.EffectiveSubject())
		}
		if got.Type != "shell.output" {
			t.Errorf("Type = %q, want the unchanged published type shell.output", got.Type)
		}
		if got.ID != "evt-1" || got.Source != "test" {
			t.Errorf("ID/Source = %q/%q, want evt-1/test", got.ID, got.Source)
		}
		payload, ok := got.Data.(map[string]interface{})
		if !ok || payload["chunk"] != "hello" {
			t.Errorf("Data = %#v, want the published payload", got.Data)
		}
	default:
		t.Fatal("handler did not invoke the EventHandler")
	}
}

// TestNATSCreateMsgHandler_StampsSubjectOnUnstampedEvent covers an event
// marshaled by a publisher that predates Subject stamping: the concrete
// message subject is still applied on arrival.
func TestNATSCreateMsgHandler_StampsSubjectOnUnstampedEvent(t *testing.T) {
	data, err := json.Marshal(&Event{ID: "evt-2", Type: "task.created", Source: "test"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	receivedCh := make(chan *Event, 1)
	b := &NATSEventBus{logger: newTestLogger(t)}
	handler := b.createMsgHandler("task.created", func(_ context.Context, e *Event) error {
		receivedCh <- e
		return nil
	})

	handler(&nats.Msg{Subject: "task.created", Data: data})

	select {
	case got := <-receivedCh:
		if got.Subject != "task.created" {
			t.Errorf("Subject = %q, want task.created", got.Subject)
		}
	default:
		t.Fatal("handler did not invoke the EventHandler")
	}
}

// TestNATSCreateMsgHandler_PanicDoesNotEscape is the regression test for the
// panic guard on the NATS receive path. The NATS client invokes the returned
// nats.MsgHandler on its own goroutine, so an unrecovered panic there would
// kill the process rather than merely truncate a delivery loop — containment
// matters even more here than on the in-memory bus.
func TestNATSCreateMsgHandler_PanicDoesNotEscape(t *testing.T) {
	data, err := json.Marshal(&Event{ID: "evt-3", Type: "task.created", Source: "test"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	b := &NATSEventBus{logger: newTestLogger(t)}
	handler := b.createMsgHandler("task.created", func(_ context.Context, e *Event) error {
		panic("handler blew up")
	})

	handlerReturnedNormally := func() (ok bool) {
		defer func() {
			if r := recover(); r != nil {
				ok = false
			}
		}()
		handler(&nats.Msg{Subject: "task.created", Data: data})
		return true
	}()

	if !handlerReturnedNormally {
		t.Fatal("nats.MsgHandler panicked into the NATS client's goroutine instead of being contained")
	}
}

// TestNATSPublish_StampsSubject pins the send side: Publish stamps the concrete
// subject onto the event so it survives the marshal/wire hop (the receive side
// then re-applies the delivering subject on top).
func TestNATSPublish_StampsSubject(t *testing.T) {
	ev := NewEvent("shell.output", "test", map[string]interface{}{})

	b := &NATSEventBus{logger: newTestLogger(t)}
	// No server here, so conn is nil and the send hop (b.conn.Publish) panics
	// on the nil dereference. The recover is deliberate: stamping happens
	// before the marshal/send, so the assertion below still holds, and if the
	// implementation were ever rearranged to stamp after sending, this test
	// would fail rather than silently pass.
	func() {
		defer func() { _ = recover() }()
		_ = b.Publish(context.Background(), "shell.output.sess-1", ev)
	}()

	if ev.Subject != "shell.output.sess-1" {
		t.Errorf("Subject = %q, want shell.output.sess-1", ev.Subject)
	}
	if ev.Type != "shell.output" {
		t.Errorf("Type = %q, want the unchanged bare type shell.output", ev.Type)
	}
}
