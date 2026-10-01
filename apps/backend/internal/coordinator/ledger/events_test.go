package ledger

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func memoryBus(t *testing.T) *bus.MemoryEventBus {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return bus.NewMemoryEventBus(log)
}

func TestSubscribe_PublishReturnsWhileTheHandlerIsBlocked(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	f.l.deps.CoordinatorForTask = func(ctx context.Context, taskID string) (string, bool, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return f.coord.ID, taskID == testConvTask, nil
	}
	f.l.Start(t.Context())
	eb := memoryBus(t)
	unsub, err := f.l.Subscribe(eb)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(unsub)
	t.Cleanup(func() { close(release) })

	published := make(chan error, 1)
	go func() {
		published <- eb.Publish(context.Background(), events.TurnStarted, turnEventData("st-1", f.at(0), nil))
	}()
	select {
	case err := <-published:
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on the ledger handler")
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never ran")
	}
}

func TestSubscribe_StartIsHandledBeforeItsCompletion(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.l.Start(t.Context())
	eb := memoryBus(t)
	unsub, err := f.l.Subscribe(eb)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(unsub)
	s := f.at(0)
	done := f.at(time.Second)
	if err := eb.Publish(context.Background(), events.TurnStarted, turnEventData("st-1", s, nil)); err != nil {
		t.Fatal(err)
	}
	if err := eb.Publish(context.Background(), events.TurnCompleted, turnEventData("st-1", s, &done)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		rows := f.turns()
		return len(rows) == 1 && rows[0].FinishedAt != nil
	})
}

func TestEnqueueEvent_FullQueueDropsAndCounts(t *testing.T) {
	f := newFixture(t)
	for range turnEventQueueSize {
		f.l.enqueueEvent(turnJob{handle: func(context.Context, *bus.Event) {}})
	}
	before := FailureCount(StageStart)
	f.l.enqueueEvent(turnJob{stage: StageStart, handle: func(context.Context, *bus.Event) {}})
	if FailureCount(StageStart) != before+1 {
		t.Fatal("dropped event not counted")
	}
}

func TestEnqueueEvent_DroppedCompletionIsCountedAsCompleteStage(t *testing.T) {
	f := newFixture(t)
	for range turnEventQueueSize {
		f.l.enqueueEvent(turnJob{handle: func(context.Context, *bus.Event) {}})
	}
	beforeComplete, beforeStart := FailureCount(StageComplete), FailureCount(StageStart)
	f.l.enqueueEvent(turnJob{stage: StageComplete, handle: func(context.Context, *bus.Event) {}})
	if FailureCount(StageComplete) != beforeComplete+1 || FailureCount(StageStart) != beforeStart {
		t.Fatal("dropped completion not counted under the complete stage")
	}
}

func TestActive_CompletionClearsAnEntryHoldingAGeneratedRowID(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	s := f.at(0)
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at)
		VALUES ('existing', ?, ?, 'st-1', 'message', ?)`, f.coord.ID, testSession, s)
	f.l.setActive(testSession, activeEntry{rowID: "generated", sessionTurnID: "st-1", startedAt: s})
	f.complete("st-1", s, f.at(time.Second))
	if got := f.l.ActiveTurnID(testSession); got != "" {
		t.Fatalf("finished turn still active as %q", got)
	}
}
