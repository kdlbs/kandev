package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func pendingAction(taskID, sessionID, action string) map[string]any {
	return map[string]any{
		"workspace_id": "ws-1", "task_id": taskID, "session_id": sessionID,
		"pending_action": action, "pending_id": "payload-key-never-used",
	}
}

func wantWakes(t *testing.T, e *wakeEnv, want ...storedWake) {
	t.Helper()
	got := e.wakes(t)
	if len(got) != len(want) {
		t.Fatalf("wakes = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wakes = %+v, want %+v", got, want)
		}
	}
}

func TestRecorder_QuestionKeyComesFromStoredState(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.question[sid] = "pending-1"
	e.start(t)
	before := wakeRecordedCounter(WakeKindQuestion)

	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", sid, "clarification"))
	wantWakes(t, e, storedWake{"t-1", "question", "pending-1", "pending"})
	if got := e.seq(); len(got) != 2 || got[0] != "updated" || got[1] != "kick:"+e.fix.c.ID {
		t.Fatalf("publish then kick order = %v", got)
	}
	if wakeRecordedCounter(WakeKindQuestion) != before+1 {
		t.Fatal("recorded counter not incremented")
	}

	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", sid, "clarification"))
	wantWakes(t, e, storedWake{"t-1", "question", "pending-1", "pending"})
	if len(e.seq()) != 2 {
		t.Fatalf("a redelivered event published or kicked again: %v", e.seq())
	}
}

func TestRecorder_PermissionRecordsEachPendingRequestInOrder(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.perms[sid] = []string{"p-b", "p-a"}
	e.start(t)
	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", sid, "permission"))
	wantWakes(t, e,
		storedWake{"t-1", "permission", "p-a", "pending"},
		storedWake{"t-1", "permission", "p-b", "pending"})
	if len(e.seq()) != 4 {
		t.Fatalf("one publish and kick per inserted wake, got %v", e.seq())
	}
}

func TestRecorder_IgnoresOtherPendingActions(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.question[sid] = "q"
	e.start(t)
	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", sid, ""))
	e.publish(t, events.SessionPendingActionChanged, map[string]any{"task_id": "t-1", "session_id": sid, "pending_action": nil})
	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", sid, "something"))
	wantWakes(t, e)
}

func TestRecorder_ErrorUsesStoredStampAndOnlyWhenActive(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.errStamp[sid] = "2026-09-30T10:00:00.123456789Z:boom"
	e.start(t)
	e.publish(t, events.TaskSessionErrorChanged, map[string]any{"task_id": "t-1", "session_id": sid, "active": false, "stamp": "payload"})
	wantWakes(t, e)
	e.publish(t, events.TaskSessionErrorChanged, map[string]any{"task_id": "t-1", "session_id": sid, "active": true, "stamp": "payload"})
	wantWakes(t, e, storedWake{"t-1", "error", "2026-09-30T10:00:00.123456789Z:boom", "pending"})
}

func TestRecorder_CompletedNeedsOnlyTheTaskID(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	e.sources.state["t-1"] = "COMPLETED"
	e.start(t)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-1", "state": "IN_PROGRESS"})
	wantWakes(t, e)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-1", "state": "COMPLETED"})
	wantWakes(t, e, storedWake{"t-1", "completed", "completed", "pending"})
}

func TestRecorder_CompletedEventWithStoredStateNotCompletedRecordsNothing(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	e.sources.state["t-1"] = "IN_PROGRESS"
	e.start(t)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-1", "state": "COMPLETED"})
	wantWakes(t, e)
}

func TestRecorder_SessionScopedEventsNeedASessionID(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.question[sid] = "q"
	e.sources.errStamp[sid] = "stamp"
	e.start(t)
	dropped := wakeDroppedCounter(dropReasonReadError) + wakeDroppedCounter(dropReasonNotOwn)
	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", "", "clarification"))
	e.publish(t, events.TaskSessionErrorChanged, map[string]any{"task_id": "t-1", "active": true})
	e.publish(t, events.SessionPendingActionChanged, pendingAction("", sid, "clarification"))
	wantWakes(t, e)
	if got := wakeDroppedCounter(dropReasonReadError) + wakeDroppedCounter(dropReasonNotOwn); got != dropped {
		t.Fatal("an event without ids must be dropped with no metric")
	}
}

func TestRecorder_NonPrimarySessionIsIgnored(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	e.sources.question["s-other"] = "q"
	e.sources.question["s-t-1"] = "q-primary"
	e.start(t)
	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", "s-other", "clarification"))
	wantWakes(t, e)
}

func TestRecorder_OnlyOwnTasksOfAutonomousCoordinatorsInWatchedWorkflows(t *testing.T) {
	e := newWakeEnv(t)
	e.fix.addTask(t, "t-msg", "wf-a")
	e.fix.addProposal(t, e.fix.c.ID, "t-msg", "message", "approved")
	e.fix.addTask(t, "t-none", "wf-a")
	sidOwn := e.ownTask(t, "t-own")
	for _, id := range []string{"t-msg", "t-none", "t-own"} {
		e.sources.state[id] = "COMPLETED"
	}
	e.start(t)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-msg", "state": "COMPLETED"})
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-none", "state": "COMPLETED"})
	wantWakes(t, e)

	setAutonomy(t, e.store, e.fix.c.ID, false)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-own", "state": "COMPLETED"})
	wantWakes(t, e)
	setAutonomy(t, e.store, e.fix.c.ID, true)

	mustExec(t, e.store, `UPDATE coordinators SET watch_scope = 'selected' WHERE id = ?`, e.fix.c.ID)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-own", "state": "COMPLETED"})
	wantWakes(t, e)
	mustExec(t, e.store, `INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, 'wf-a', 'ws-1', ?)`, e.fix.c.ID, time.Now().UTC())
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-own", "state": "COMPLETED"})
	wantWakes(t, e, storedWake{"t-own", "completed", "completed", "pending"})
	_ = sidOwn
}

func TestRecorder_ReadErrorsDropTheEventAndCount(t *testing.T) {
	for _, method := range []string{"PrimarySessionID", "PendingQuestionID"} {
		e := newWakeEnv(t)
		sid := e.ownTask(t, "t-1")
		e.sources.question[sid] = "q"
		e.sources.failWith(method, errBoom)
		e.start(t)
		before := wakeDroppedCounter(dropReasonReadError)
		e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", sid, "clarification"))
		wantWakes(t, e)
		if wakeDroppedCounter(dropReasonReadError) != before+1 {
			t.Fatalf("%s: read_error not counted", method)
		}
	}
}

func TestRecorder_BlankKeyIsNotAnEpisode(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.question[sid] = "   "
	e.start(t)
	before := wakeDroppedCounter(dropReasonWriteError)
	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", sid, "clarification"))
	wantWakes(t, e)
	if wakeDroppedCounter(dropReasonWriteError) != before {
		t.Fatal("a blank key must not count as a write error")
	}
}

func TestRecorder_CapCountsDroppedWake(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	e.sources.state["t-1"] = "COMPLETED"
	seedPendingWakes(t, e.fix, 200)
	e.start(t)
	before := wakeDroppedCounter(dropReasonCap)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-1", "state": "COMPLETED"})
	if wakeDroppedCounter(dropReasonCap) != before+1 {
		t.Fatal("cap not counted")
	}
	if len(e.seq()) != 0 {
		t.Fatalf("a capped wake published or kicked: %v", e.seq())
	}
}

func TestRecordWake_DeletedCoordinatorCountsNotFound(t *testing.T) {
	e := newWakeEnv(t)
	before := wakeDroppedCounter(dropReasonNotFound)
	if _, err := e.svc.RecordWake(context.Background(), "gone", "t-1", WakeKindCompleted, "completed"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if wakeDroppedCounter(dropReasonNotFound) != before+1 {
		t.Fatal("not_found not counted")
	}
}

func TestRecordWake_RefusalsCountTheirReason(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	ctx := context.Background()
	notOwn := wakeDroppedCounter(dropReasonNotOwn)
	off := wakeDroppedCounter(dropReasonAutonomyOff)
	write := wakeDroppedCounter(dropReasonWriteError)
	if out, err := e.svc.RecordWake(ctx, e.fix.c.ID, "t-none", WakeKindCompleted, "completed"); err != nil || out != WakeNotOwn {
		t.Fatalf("not own = %v, %v", out, err)
	}
	if _, err := e.svc.RecordWake(ctx, e.fix.c.ID, "t-1", WakeKindCompleted, " "); err == nil {
		t.Fatal("blank key must error")
	}
	setAutonomy(t, e.store, e.fix.c.ID, false)
	if out, err := e.svc.RecordWake(ctx, e.fix.c.ID, "t-1", WakeKindCompleted, "completed"); err != nil || out != WakeAutonomyOff {
		t.Fatalf("autonomy off = %v, %v", out, err)
	}
	if wakeDroppedCounter(dropReasonNotOwn) != notOwn+1 || wakeDroppedCounter(dropReasonAutonomyOff) != off+1 || wakeDroppedCounter(dropReasonWriteError) != write+1 {
		t.Fatal("refusal reasons not counted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.svc.RecordWake(cancelled, e.fix.c.ID, "t-1", WakeKindCompleted, "completed"); err == nil {
		t.Fatal("cancelled ctx must error")
	}
	if wakeDroppedCounter(dropReasonWriteError) != write+1 {
		t.Fatal("a cancelled ctx is shutdown, not a write error")
	}
}

func TestKick_NilSafePanicAndErrorAreContained(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	e.svc.SetKick(nil)
	if out, err := e.svc.RecordWake(context.Background(), e.fix.c.ID, "t-1", WakeKindCompleted, "a"); err != nil || out != WakeInserted {
		t.Fatalf("nil kick: %v, %v", out, err)
	}
	e.svc.SetKick(func(context.Context, string) error { panic("kick exploded") })
	if out, err := e.svc.RecordWake(context.Background(), e.fix.c.ID, "t-1", WakeKindCompleted, "b"); err != nil || out != WakeInserted {
		t.Fatalf("panicking kick: %v, %v", out, err)
	}
	e.svc.SetKick(func(context.Context, string) error { return errBoom })
	if out, err := e.svc.RecordWake(context.Background(), e.fix.c.ID, "t-1", WakeKindCompleted, "c"); err != nil || out != WakeInserted {
		t.Fatalf("failing kick: %v, %v", out, err)
	}
}

func publishStall(t *testing.T, e *wakeEnv, taskID string, lastEventAt time.Time) {
	t.Helper()
	e.publish(t, events.TaskStalled, map[string]any{
		"task_id": taskID, "workspace_id": "ws-1", "stalled_for": "2h0m0s",
		"last_event_at": lastEventAt.Format(time.RFC3339Nano),
	})
}

func subscribeStalls(t *testing.T, e *wakeEnv) {
	t.Helper()
	sub, err := SubscribeTaskStalled(e.bus, e.svc, newTestLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
}

func TestRecorder_StallKeyIsTheStoredFixedWidthLastEventAt(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	subscribeStalls(t, e)
	e.start(t)
	last := time.Date(2026, 9, 30, 10, 0, 0, 120_000_000, time.UTC)
	publishStall(t, e, "t-1", last)
	wantWakes(t, e, storedWake{"t-1", "stall", "2026-09-30T10:00:00.120000000Z", "pending"})
	publishStall(t, e, "t-1", last)
	if len(e.wakes(t)) != 1 {
		t.Fatal("a redelivered stall event stored a second wake")
	}
	publishStall(t, e, "t-1", last.Add(time.Hour))
	if len(e.wakes(t)) != 2 {
		t.Fatal("a later stall is a new episode")
	}
}

func TestRecorder_StallCurrency(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	subscribeStalls(t, e)
	e.start(t)
	later := time.Now().UTC().Add(time.Hour)
	e.sources.last["t-1"] = &later
	publishStall(t, e, "t-1", time.Now().UTC().Add(-2*time.Hour))
	wantWakes(t, e)

	before := wakeDroppedCounter(dropReasonReadError)
	e.sources.failWith("LastActivityAt", errBoom)
	publishStall(t, e, "t-1", time.Now().UTC().Add(-time.Hour))
	wantWakes(t, e)
	if wakeDroppedCounter(dropReasonReadError) != before+1 {
		t.Fatal("a failing LastActivityAt is a read error, not a current stall")
	}
}

func TestRecorder_StallHookIsOffUntilTheRecorderStarts(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	subscribeStalls(t, e)
	publishStall(t, e, "t-1", time.Now().UTC().Add(-time.Hour))
	wantWakes(t, e)
}

func TestRecorder_StallHookPanicIsRecovered(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	subscribeStalls(t, e)
	called := 0
	e.svc.SetStallWakeHook(func(context.Context, string, string) { called++; panic("hook exploded") })
	publishStall(t, e, "t-1", time.Now().UTC().Add(-time.Hour))
	if called != 1 {
		t.Fatalf("hook calls = %d", called)
	}
}

func TestStopWakeRecorder_UnsubscribesAndIsIdempotent(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	subscribeStalls(t, e)
	e.sources.state["t-1"] = "COMPLETED"
	e.svc.StartWakeRecorder(e.bus, e.sources)
	e.svc.StopWakeRecorder()
	e.svc.StopWakeRecorder()
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-1", "state": "COMPLETED"})
	publishStall(t, e, "t-1", time.Now().UTC().Add(-time.Hour))
	wantWakes(t, e)
	e.svc.StartWakeRecorder(e.bus, e.sources)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-1", "state": "COMPLETED"})
	wantWakes(t, e)
}

func TestStopWakeRecorder_WaitsForKickInFlightWithoutHoldingTheMutex(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	e.sources.state["t-1"] = "COMPLETED"
	inKick := make(chan struct{})
	releaseKick := make(chan struct{})
	e.svc.SetKick(func(context.Context, string) error {
		close(inKick)
		<-releaseKick
		return nil
	})
	e.start(t)

	published := make(chan struct{})
	go func() {
		defer close(published)
		e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-1", "state": "COMPLETED"})
	}()
	<-inKick

	stopped := make(chan struct{})
	go func() { defer close(stopped); e.svc.StopWakeRecorder() }()

	// While Stop waits for the kick, the Service mutex must be free.
	swapped := make(chan struct{})
	go func() {
		defer close(swapped)
		e.svc.SetKick(nil)
		e.svc.SetStallWakeHook(nil)
	}()
	<-swapped
	select {
	case <-stopped:
		t.Fatal("StopWakeRecorder returned while a kick was in flight")
	default:
	}
	close(releaseKick)
	<-stopped
	<-published
	wantWakes(t, e, storedWake{"t-1", "completed", "completed", "pending"})
}

func TestStopWakeRecorder_StallHookInFlightIsJoined(t *testing.T) {
	e := newWakeEnv(t)
	inHook := make(chan struct{})
	releaseHook := make(chan struct{})
	e.svc.SetStallWakeHook(func(context.Context, string, string) {
		close(inHook)
		<-releaseHook
	})
	e.svc.wakeMu.Lock()
	e.svc.wakeSources = e.sources
	e.svc.wakeMu.Unlock()
	done := make(chan struct{})
	go func() { defer close(done); e.svc.runStallWakeHook(context.Background(), "ws-1", "t-1") }()
	<-inHook
	stopped := make(chan struct{})
	go func() { defer close(stopped); e.svc.StopWakeRecorder() }()
	select {
	case <-stopped:
		t.Fatal("StopWakeRecorder returned while a stall hook was in flight")
	default:
	}
	close(releaseHook)
	<-stopped
	<-done
	called := false
	e.svc.SetStallWakeHook(func(context.Context, string, string) { called = true })
	e.svc.runStallWakeHook(context.Background(), "ws-1", "t-1")
	if called {
		t.Fatal("a hook call after stop must be a no-op")
	}
}

// failingSubscribeBus refuses one subject.
type failingSubscribeBus struct {
	bus.EventBus
	refuse string
}

func (b failingSubscribeBus) Subscribe(subject string, h bus.EventHandler) (bus.Subscription, error) {
	if subject == b.refuse {
		return nil, errBoom
	}
	return b.EventBus.Subscribe(subject, h)
}

func TestStartWakeRecorder_ASubscribeFailureKeepsTheOthers(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.question[sid] = "q"
	e.sources.state["t-1"] = "COMPLETED"
	e.svc.StartWakeRecorder(failingSubscribeBus{EventBus: e.bus, refuse: events.TaskStateChanged}, e.sources)
	t.Cleanup(e.svc.StopWakeRecorder)
	e.publish(t, events.TaskStateChanged, map[string]any{"task_id": "t-1", "state": "COMPLETED"})
	wantWakes(t, e)
	e.publish(t, events.SessionPendingActionChanged, pendingAction("t-1", sid, "clarification"))
	wantWakes(t, e, storedWake{"t-1", "question", "q", "pending"})
}
