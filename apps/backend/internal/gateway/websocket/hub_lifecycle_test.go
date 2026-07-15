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
			returned := make(chan struct{})
			go func() {
				lifecycleCall(newTestClient(name))
				close(returned)
			}()

			select {
			case <-returned:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("client lifecycle call blocked after hub shutdown")
			}
		})
	}
}
