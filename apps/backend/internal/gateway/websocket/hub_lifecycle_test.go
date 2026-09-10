package websocket

import (
	"context"
	"testing"
	"time"
)

func TestHub_ClientLifecycleCallsReturnAfterShutdown(t *testing.T) {
	h := newTestHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		h.Run(ctx)
	}()
	cancel()
	<-hubDone

	for name, lifecycleCall := range map[string]func(*Client){
		"register":   h.Register,
		"unregister": h.Unregister,
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestClient(name)
			returned := make(chan struct{})
			go func() {
				lifecycleCall(client)
				close(returned)
			}()

			select {
			case <-returned:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("client lifecycle call blocked after hub shutdown")
			}
			if !client.closed {
				t.Fatal("client send channel remained open after hub shutdown")
			}
		})
	}
}

func TestHubClientDisconnectNotifiesListenerOnce(t *testing.T) {
	h := newTestHub(t)
	client := newTestClient("connection-a")
	h.clients[client] = true
	disconnected := make(chan string, 1)
	h.SetClientDisconnectListener(func(connectionID string) {
		disconnected <- connectionID
	})

	h.removeClient(client)
	h.removeClient(client)

	select {
	case connectionID := <-disconnected:
		if connectionID != client.ID {
			t.Fatalf("disconnect notification = %q, want %q", connectionID, client.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect listener was not called")
	}
	select {
	case duplicate := <-disconnected:
		t.Fatalf("duplicate disconnect notification = %q", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}
