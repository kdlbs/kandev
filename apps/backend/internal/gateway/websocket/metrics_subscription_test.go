package websocket

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type metricsInterestRecorder struct {
	subs   []string
	unsubs []string
}

func (r *metricsInterestRecorder) MetricsSubscribe(clientID string) {
	r.subs = append(r.subs, clientID)
}

func (r *metricsInterestRecorder) MetricsUnsubscribe(clientID string) {
	r.unsubs = append(r.unsubs, clientID)
}

func TestSystemMetricsSubscriptionBroadcastsOnlyToSubscribers(t *testing.T) {
	h := newTestHub(t)
	c1 := newTestClient("c1")
	c2 := newTestClient("c2")
	registerTestClient(h, c1)
	registerTestClient(h, c2)

	h.SubscribeToSystemMetrics(c1)

	msg, err := ws.NewNotification(ws.ActionSystemMetricsUpdated, map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSystemMetrics(msg)

	if !clientReceived(c1) {
		t.Fatal("subscribed client did not receive metrics update")
	}
	if clientReceived(c2) {
		t.Fatal("unsubscribed client received metrics update")
	}
}

func TestSystemMetricsInterestTracksClientLifecycle(t *testing.T) {
	h := newTestHub(t)
	rec := &metricsInterestRecorder{}
	h.SetSystemMetricsInterestTracker(rec)
	c := newTestClient("c1")
	registerTestClient(h, c)

	h.SubscribeToSystemMetrics(c)
	h.SubscribeToSystemMetrics(c)
	if len(rec.subs) != 1 {
		t.Fatalf("subscribe calls=%d, want 1", len(rec.subs))
	}

	h.removeClient(c)
	if len(rec.unsubs) != 1 {
		t.Fatalf("unsubscribe calls=%d, want 1", len(rec.unsubs))
	}
}

func TestSystemMetricsInterestTracksHubShutdown(t *testing.T) {
	h := newTestHub(t)
	rec := &metricsInterestRecorder{}
	h.SetSystemMetricsInterestTracker(rec)
	c := newTestClient("c1")
	registerTestClient(h, c)

	h.SubscribeToSystemMetrics(c)
	h.closeAllClients()

	if len(rec.unsubs) != 1 {
		t.Fatalf("unsubscribe calls=%d, want 1", len(rec.unsubs))
	}
}

func TestHandleSystemMetricsSubscribe(t *testing.T) {
	h := newTestHub(t)
	rec := &metricsInterestRecorder{}
	h.SetSystemMetricsInterestTracker(rec)
	c := newTestClient("c1")
	c.hub = h

	msg, _ := ws.NewRequest("req-1", ws.ActionSystemMetricsSubscribe, map[string]any{})
	c.handleMessage(msg)

	if len(rec.subs) != 1 {
		t.Fatalf("subscribe calls=%d, want 1", len(rec.subs))
	}
	select {
	case data := <-c.send:
		var got ws.Message
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.Type != ws.MessageTypeResponse || got.Action != ws.ActionSystemMetricsSubscribe {
			t.Fatalf("unexpected response type/action: %s %s", got.Type, got.Action)
		}
	default:
		t.Fatal("expected subscribe response")
	}
}
