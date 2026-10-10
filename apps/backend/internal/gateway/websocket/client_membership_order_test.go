package websocket

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestReadPumpSessionMembershipFollowsWireOrder(t *testing.T) {
	connection := newConnectedClient(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	connection.hub.authPolicy.Subscriptions.Session = func(context.Context, string) error {
		close(entered)
		<-release
		return nil
	}
	write := func(id, action string) {
		t.Helper()
		message := ws.Message{ID: id, Type: ws.MessageTypeRequest, Action: action,
			Payload: mustJSON(t, map[string]string{"session_id": "session-ordered"})}
		if err := connection.browser.WriteJSON(message); err != nil {
			t.Fatal(err)
		}
	}
	write("subscribe", ws.ActionSessionSubscribe)
	select {
	case <-entered:
	case <-time.After(wsTestTimeout):
		t.Fatal("subscription authorization did not start")
	}
	write("unsubscribe", ws.ActionSessionUnsubscribe)
	replies := make(chan string, 2)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for received := 0; received < 2; {
			_, raw, err := connection.browser.ReadMessage()
			if err != nil {
				return
			}
			for _, part := range strings.Split(string(raw), "\n") {
				var reply ws.Message
				if json.Unmarshal([]byte(part), &reply) == nil && reply.Type == ws.MessageTypeResponse {
					replies <- reply.ID
					received++
				}
			}
		}
	}()
	t.Cleanup(func() {
		_ = connection.browser.Close()
		select {
		case <-readerDone:
		case <-time.After(wsTestTimeout):
			t.Error("reply reader did not stop")
		}
	})
	select {
	case id := <-replies:
		t.Fatalf("%s overtook the blocked subscription", id)
	case <-time.After(250 * time.Millisecond):
	}
	unblock()
	for _, expected := range []string{"subscribe", "unsubscribe"} {
		select {
		case id := <-replies:
			if id != expected {
				t.Fatalf("reply = %s, want %s", id, expected)
			}
		case <-time.After(wsTestTimeout):
			t.Fatalf("missing %s acknowledgement", expected)
		}
	}
}

func TestReadPumpSessionSnapshotDoesNotBlockUnsubscribe(t *testing.T) {
	connection := newConnectedClient(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	connection.hub.SetSessionDataProvider(func(context.Context, string) ([]*ws.Message, error) {
		defer close(finished)
		close(entered)
		<-release
		return nil, nil
	})
	response := connection.request(t, "subscribe", ws.ActionSessionSubscribe,
		map[string]string{"session_id": "session-snapshot"})
	if response.ID != "subscribe" || response.Type != ws.MessageTypeResponse {
		t.Fatalf("unexpected subscribe response: %+v", response)
	}
	select {
	case <-entered:
	case <-time.After(wsTestTimeout):
		t.Fatal("session snapshot did not start")
	}
	response = connection.request(t, "unsubscribe", ws.ActionSessionUnsubscribe,
		map[string]string{"session_id": "session-snapshot"})
	if response.ID != "unsubscribe" || response.Type != ws.MessageTypeResponse {
		t.Fatalf("unexpected unsubscribe response: %+v", response)
	}
	unblock()
	select {
	case <-finished:
	case <-time.After(wsTestTimeout):
		t.Fatal("session snapshot did not stop")
	}
}
