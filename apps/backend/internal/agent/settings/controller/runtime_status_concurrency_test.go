package controller

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
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

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.2
func TestRuntimeStatusCallerCancellationPreservesSharedSourceAndCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestController(map[string]agents.Agent{"gemini": agents.NewGemini()})
		started, release := make(chan struct{}), make(chan struct{})
		var calls atomic.Int32
		c.SetRuntimeUpdateStatusResolver(func(ctx context.Context, _ string) (string, error) {
			calls.Add(1)
			close(started)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-release:
				return "9.0.0", nil
			}
		})
		ctx, cancel := context.WithCancel(context.Background())
		first := make(chan struct{})
		go func() { _, _ = c.ListAgentUpdateStatuses(ctx); close(first) }()
		<-started
		second := make(chan dto.AgentUpdateCheckState, 1)
		go func() {
			response, _ := c.ListAgentUpdateStatuses(context.Background())
			second <- response.Statuses[0].CheckState
		}()
		synctest.Wait()
		cancel()
		<-first
		close(release)
		if state := <-second; state != dto.AgentUpdateCheckStateUpdateAvailable {
			t.Fatalf("remaining subscriber lost shared source: %s", state)
		}
		response, _ := c.ListAgentUpdateStatuses(context.Background())
		if response.Statuses[0].LatestVersion != "9.0.0" || calls.Load() != 1 {
			t.Fatalf("caller cancellation poisoned cache: %+v; calls=%d", response, calls.Load())
		}
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
	})
}
