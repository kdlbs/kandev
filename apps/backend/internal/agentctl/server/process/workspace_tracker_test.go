package process

import (
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error", // Suppress logs during tests
		Format: "console",
	})
	return log
}

func TestWorkspaceTracker_SubscribeWorkspaceStream(t *testing.T) {
	log := newTestLogger()
	workDir := t.TempDir()

	wt := NewWorkspaceTracker(workDir, log)
	if wt == nil {
		t.Fatal("expected non-nil WorkspaceTracker")
	}

	// Subscribe to workspace stream
	sub := wt.SubscribeWorkspaceStream()
	if sub == nil {
		t.Fatal("expected non-nil subscriber channel")
	}

	// Verify subscriber is registered
	wt.workspaceSubMu.RLock()
	_, exists := wt.workspaceStreamSubscribers[sub]
	wt.workspaceSubMu.RUnlock()

	if !exists {
		t.Error("expected subscriber to be registered")
	}

	// Unsubscribe
	wt.UnsubscribeWorkspaceStream(sub)

	// Verify subscriber is removed
	wt.workspaceSubMu.RLock()
	_, exists = wt.workspaceStreamSubscribers[sub]
	wt.workspaceSubMu.RUnlock()

	if exists {
		t.Error("expected subscriber to be unregistered")
	}
}

func TestWorkspaceTracker_NotifyWorkspaceStreamMessage(t *testing.T) {
	log := newTestLogger()
	workDir := t.TempDir()

	wt := NewWorkspaceTracker(workDir, log)

	// Subscribe to workspace stream
	sub := wt.SubscribeWorkspaceStream()
	defer wt.UnsubscribeWorkspaceStream(sub)

	// Send a message
	msg := types.NewWorkspaceShellOutput("test output")
	wt.NotifyWorkspaceStreamMessage(msg)

	// Receive the message
	select {
	case received := <-sub:
		if received.Type != types.WorkspaceMessageTypeShellOutput {
			t.Errorf("expected type %q, got %q", types.WorkspaceMessageTypeShellOutput, received.Type)
		}
		if received.Data != "test output" {
			t.Errorf("expected data 'test output', got %q", received.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for message")
	}
}

func TestWorkspaceTracker_ConcurrentSubscribe(t *testing.T) {
	log := newTestLogger()
	workDir := t.TempDir()

	wt := NewWorkspaceTracker(workDir, log)

	var wg sync.WaitGroup
	subscribers := make([]types.WorkspaceStreamSubscriber, 10)

	// Concurrently subscribe
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			subscribers[idx] = wt.SubscribeWorkspaceStream()
		}(i)
	}
	wg.Wait()

	// Verify all subscribers are registered
	wt.workspaceSubMu.RLock()
	count := len(wt.workspaceStreamSubscribers)
	wt.workspaceSubMu.RUnlock()

	if count != 10 {
		t.Errorf("expected 10 subscribers, got %d", count)
	}

	// Concurrently unsubscribe
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			wt.UnsubscribeWorkspaceStream(subscribers[idx])
		}(i)
	}
	wg.Wait()

	// Verify all subscribers are removed
	wt.workspaceSubMu.RLock()
	count = len(wt.workspaceStreamSubscribers)
	wt.workspaceSubMu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 subscribers, got %d", count)
	}
}

func TestWorkspaceTracker_MultipleSubscribers(t *testing.T) {
	log := newTestLogger()
	workDir := t.TempDir()

	wt := NewWorkspaceTracker(workDir, log)

	// Create multiple subscribers
	sub1 := wt.SubscribeWorkspaceStream()
	sub2 := wt.SubscribeWorkspaceStream()
	defer wt.UnsubscribeWorkspaceStream(sub1)
	defer wt.UnsubscribeWorkspaceStream(sub2)

	// Send a message
	msg := types.NewWorkspaceShellOutput("broadcast test")
	wt.NotifyWorkspaceStreamMessage(msg)

	// Both subscribers should receive the message
	for i, sub := range []types.WorkspaceStreamSubscriber{sub1, sub2} {
		select {
		case received := <-sub:
			if received.Data != "broadcast test" {
				t.Errorf("subscriber %d: expected data 'broadcast test', got %q", i, received.Data)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d: timeout waiting for message", i)
		}
	}
}

