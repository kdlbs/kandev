package config

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type blockingActivityLogger struct {
	calls        atomic.Int32
	firstEntered chan struct{}
	secondSeen   chan struct{}
	release      chan struct{}
	once         sync.Once
}

func (l *blockingActivityLogger) LogActivity(
	_ context.Context, _, _, _, _, _, _, _ string,
) {
	switch l.calls.Add(1) {
	case 1:
		l.once.Do(func() { close(l.firstEntered) })
		<-l.release
	case 2:
		close(l.secondSeen)
	}
}

func (l *blockingActivityLogger) LogActivityWithRun(
	ctx context.Context, wsID, actorType, actorID, action, targetType, targetID, details, _, _ string,
) {
	l.LogActivity(ctx, wsID, actorType, actorID, action, targetType, targetID, details)
}

// TestApplyImport_SerializesConcurrentRequests prevents two imports for the
// same service from validating and writing hierarchy state at the same time.
func TestApplyImport_SerializesConcurrentRequests(t *testing.T) {
	env := newTestEnv(t)
	logger := &blockingActivityLogger{
		firstEntered: make(chan struct{}),
		secondSeen:   make(chan struct{}),
		release:      make(chan struct{}),
	}
	env.svc.activity = logger
	t.Cleanup(func() {
		select {
		case <-logger.release:
		default:
			close(logger.release)
		}
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := env.svc.ApplyImport(context.Background(), testWorkspaceID, &ConfigBundle{
			Agents: []AgentConfig{hierarchyAgent("alice", "")},
		})
		firstDone <- err
	}()
	<-logger.firstEntered

	secondDone := make(chan error, 1)
	go func() {
		_, err := env.svc.ApplyImport(context.Background(), testWorkspaceID, &ConfigBundle{
			Agents: []AgentConfig{hierarchyAgent("bob", "")},
		})
		secondDone <- err
	}()

	secondEntered := false
	select {
	case <-logger.secondSeen:
		secondEntered = true
	case <-time.After(100 * time.Millisecond):
	}

	close(logger.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first import: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second import: %v", err)
	}
	if secondEntered {
		t.Fatal("second import entered activity logging before the first import released")
	}
}
