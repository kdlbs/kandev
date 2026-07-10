//go:build race

package websocket

import (
	"context"
	"sync"
	"testing"
)

// TestDispatcher_ConcurrentRegisterAndDispatch exercises the Dispatcher
// from many goroutines at once. Its purpose is to surface a data race on
// the underlying handlers map when run under the race detector; the
// //go:build race tag scopes the test to `go test -race` runs (which CI
// uses for the backend) so the stress is not paid on plain `go test`
// invocations where it would add wall time without observing the race.
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
