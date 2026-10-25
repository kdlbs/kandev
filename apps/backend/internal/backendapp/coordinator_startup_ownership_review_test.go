package backendapp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoordinatorStartupStopCancelsAndJoinsBeforeLaterHooks(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	var later atomic.Int32
	stop := startCoordinatorBackgroundPass(context.Background(), time.Now(), []func(context.Context, time.Time){
		func(ctx context.Context, _ time.Time) { close(entered); <-ctx.Done(); close(exited) },
		func(context.Context, time.Time) { later.Add(1) },
	})
	<-entered
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("startup stop did not cancel and join")
	}
	select {
	case <-exited:
	default:
		t.Fatal("stop returned before hook exited")
	}
	if later.Load() != 0 {
		t.Fatal("startup admitted another hook after stop")
	}
	stop()
}
