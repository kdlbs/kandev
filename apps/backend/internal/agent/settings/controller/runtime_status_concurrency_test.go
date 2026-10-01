package controller

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/agent/agents"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.2
func TestRuntimeStatusConcurrentConsumersShareSourceLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestController(map[string]agents.Agent{"gemini": agents.NewGemini()})
		release := make(chan struct{})
		var calls atomic.Int32
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { calls.Add(1); <-release; return "9.0.0", nil })
		done := make(chan struct{}, 2)
		for range 2 {
			go func() { _, _ = c.ListAgentUpdateStatuses(context.Background()); done <- struct{}{} }()
		}
		synctest.Wait()
		if calls.Load() != 1 {
			t.Errorf("concurrent source lookups = %d, want 1", calls.Load())
		}
		close(release)
		<-done
		<-done
	})
}

func TestRuntimeStatusCancelledWaiterDoesNotWaitForSourceSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestController(map[string]agents.Agent{"gemini": agents.NewGemini()})
		c.runtimeUpdateStatusLookup = make(chan struct{}, runtimeUpdateStatusMaxConcurrent)
		for range runtimeUpdateStatusMaxConcurrent {
			c.runtimeUpdateStatusLookup <- struct{}{}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		done := make(chan struct{})
		go func() { _, _ = c.ListAgentUpdateStatuses(ctx); close(done) }()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Error("cancelled status blocked on source slot")
		}
		for range runtimeUpdateStatusMaxConcurrent {
			<-c.runtimeUpdateStatusLookup
		}
		<-done
	})
}
