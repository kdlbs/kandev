package coordinator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// callLog records the coordinators a duty was called with, in order.
type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) duty(name string) BackstopDuty {
	return func(_ context.Context, id string) error {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.calls = append(l.calls, name+":"+id)
		return nil
	}
}

func (l *callLog) got() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (e *wakeEnv) pass(t *testing.T) {
	t.Helper()
	e.svc.wakeMu.Lock()
	e.svc.wakeSources = e.sources
	e.svc.wakeMu.Unlock()
	e.svc.backstop.runPass(context.Background())
}

func TestBackstop_RecordsEveryKindOnceAndASecondPassWritesNothing(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.question[sid] = "q1"
	e.sources.perms[sid] = []string{"p1"}
	e.sources.errStamp[sid] = "e1"
	e.sources.state["t-1"] = "COMPLETED"
	stallAt := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	mustExec(t, e.store, `INSERT INTO coordinator_stalls (task_id, workspace_id, detected_at, last_event_at, stalled_for_ms) VALUES ('t-1', 'ws-1', ?, ?, 7200000)`, stallAt.Add(time.Hour), stallAt)

	e.pass(t)
	want := []storedWake{
		{"t-1", "completed", "completed", "pending"},
		{"t-1", "error", "e1", "pending"},
		{"t-1", "permission", "p1", "pending"},
		{"t-1", "question", "q1", "pending"},
		{"t-1", "stall", "2026-09-30T09:00:00.000000000Z", "pending"},
	}
	wantWakes(t, e, want...)
	notes := len(e.seq())
	if notes != 10 {
		t.Fatalf("one publish and kick per wake, got %v", e.seq())
	}

	e.pass(t)
	wantWakes(t, e, want...)
	if len(e.seq()) != notes {
		t.Fatalf("a second pass published or kicked again: %v", e.seq())
	}
}

func TestBackstop_ReadErrorAbandonsTheCoordinatorAndSkipsDeliver(t *testing.T) {
	e := newWakeEnv(t)
	sid := e.ownTask(t, "t-1")
	e.sources.question[sid] = "q1"
	e.sources.state["t-1"] = "COMPLETED"
	e.sources.failWith("TaskState", errBoom)
	log := &callLog{}
	e.svc.SetBackstopHooks(Hooks{TurnDuties: log.duty("turn"), Lowering: log.duty("low"), Deliver: log.duty("deliver")})
	before := backstopSkippedTotal.Value()

	e.pass(t)
	wantWakes(t, e)
	if backstopSkippedTotal.Value() != before+1 {
		t.Fatal("skipped counter not incremented")
	}
	id := e.fix.c.ID
	if !equalStrings(log.got(), []string{"turn:" + id, "low:" + id}) {
		t.Fatalf("calls = %v, want turn and lowering but no deliver", log.got())
	}

	e.sources.failWith("TaskState", nil)
	e.pass(t)
	if len(e.wakes(t)) != 2 {
		t.Fatalf("a recovered read should record, got %+v", e.wakes(t))
	}
	if got := log.got(); got[len(got)-1] != "deliver:"+id {
		t.Fatalf("deliver should follow recording, got %v", got)
	}
}

func TestBackstop_OneCoordinatorFailingDoesNotStopTheOthers(t *testing.T) {
	e := newWakeEnv(t)
	other := newTestCoordinator(t, e.store, "ws-2")
	setAutonomy(t, e.store, other.ID, true)
	e.ownTask(t, "t-1")
	e.sources.failWith("TaskState", errBoom)
	log := &callLog{}
	e.svc.SetBackstopHooks(Hooks{Deliver: log.duty("deliver")})
	e.pass(t)
	if got := log.got(); len(got) == 0 {
		t.Fatal("the coordinator with nothing to read was not visited")
	}
}

func TestBackstop_AutonomyOffWithOpenTurnRunsTurnDutiesOnly(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	e.sources.state["t-1"] = "COMPLETED"
	setAutonomy(t, e.store, e.fix.c.ID, false)
	now := time.Now().UTC()
	mustExec(t, e.store, `INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at)
		VALUES ('turn-1', ?, 'conv', 's', 0, 0, ?)`, e.fix.c.ID, now)
	log := &callLog{}
	e.svc.SetBackstopHooks(Hooks{TurnDuties: log.duty("turn"), Lowering: log.duty("low"), Deliver: log.duty("deliver")})
	e.pass(t)
	id := e.fix.c.ID
	if !equalStrings(log.got(), []string{"turn:" + id, "low:" + id}) {
		t.Fatalf("calls = %v", log.got())
	}
	wantWakes(t, e)
}

func TestBackstop_VisitSetIsTheOrderedUnion(t *testing.T) {
	e := newWakeEnv(t)
	on := newTestCoordinator(t, e.store, "ws-2")
	setAutonomy(t, e.store, on.ID, true)
	withTurn := newTestCoordinator(t, e.store, "ws-3")
	withClaim := newTestCoordinator(t, e.store, "ws-4")
	idle := newTestCoordinator(t, e.store, "ws-5")
	oldTurn := newTestCoordinator(t, e.store, "ws-6")
	now := time.Now().UTC()
	mustExec(t, e.store, `INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, outcome, started_at, finished_at)
		VALUES ('turn-recent', ?, 'c', 's', 0, 0, 'done', ?, ?)`, withTurn.ID, now.Add(-time.Hour), now.Add(-time.Minute))
	mustExec(t, e.store, `INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, outcome, started_at, finished_at)
		VALUES ('turn-old', ?, 'c', 's', 0, 0, 'done', ?, ?)`, oldTurn.ID, now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	e.fix.addTask(t, "t-claim", "wf-a")
	e.fix.addProposal(t, withClaim.ID, "t-claim", "create_task", "approved")
	mustExec(t, e.store, `UPDATE coordinator_proposals SET claimed_automatically = 1 WHERE coordinator_id = ?`, withClaim.ID)

	want := []string{e.fix.c.ID, on.ID, withTurn.ID, withClaim.ID}
	sortStrings(want)
	got := e.svc.backstop.visitSet(context.Background(), now)
	if !equalStrings(got, want) {
		t.Fatalf("visit set = %v, want %v (idle %s and old-turn %s excluded)", got, want, idle.ID, oldTurn.ID)
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func TestBackstop_DutyErrorAndPanicAreIsolated(t *testing.T) {
	e := newWakeEnv(t)
	log := &callLog{}
	e.svc.SetBackstopHooks(Hooks{
		TurnDuties: func(context.Context, string) error { return errBoom },
		Lowering:   func(context.Context, string) error { panic("lowering exploded") },
		Deliver:    log.duty("deliver"),
	})
	before := backstopSkippedTotal.Value()
	e.pass(t)
	if !equalStrings(log.got(), []string{"deliver:" + e.fix.c.ID}) {
		t.Fatalf("later duties must still run, got %v", log.got())
	}
	if backstopSkippedTotal.Value() != before+2 {
		t.Fatalf("skipped delta = %d, want 2", backstopSkippedTotal.Value()-before)
	}
}

func TestBackstop_HooksMergePerFieldAndAreIgnoredAfterStart(t *testing.T) {
	e := newWakeEnv(t)
	first, second := &callLog{}, &callLog{}
	e.svc.SetBackstopHooks(Hooks{TurnDuties: first.duty("turn"), Deliver: first.duty("deliver-1")})
	e.svc.SetBackstopHooks(Hooks{Deliver: second.duty("deliver-2")})
	e.pass(t)
	id := e.fix.c.ID
	if !equalStrings(first.got(), []string{"turn:" + id}) || !equalStrings(second.got(), []string{"deliver-2:" + id}) {
		t.Fatalf("merge: first=%v second=%v", first.got(), second.got())
	}

	e.svc.backstop.interval = time.Hour
	e.svc.StartWakeBackstop(context.Background())
	t.Cleanup(e.svc.StopWakeBackstop)
	late := &callLog{}
	e.svc.SetBackstopHooks(Hooks{Deliver: late.duty("late")})
	e.pass(t)
	if len(late.got()) != 0 {
		t.Fatalf("a hook set after Start ran: %v", late.got())
	}
}

func TestBackstop_PassPanicIsRecoveredAndTheLoopContinues(t *testing.T) {
	e := newWakeEnv(t)
	var calls atomic.Int32
	e.svc.SetBackstopHooks(Hooks{TurnDuties: func(context.Context, string) error {
		calls.Add(1)
		return nil
	}})
	e.svc.backstop.interval = 10 * time.Millisecond
	// A nil store read panics the pass; restore it afterwards.
	e.svc.backstop.svc = &Service{logger: e.svc.logger}
	e.svc.StartWakeBackstop(context.Background())
	before := backstopSkippedTotal.Value()
	deadline := time.After(5 * time.Second)
	for backstopSkippedTotal.Value() < before+2 {
		select {
		case <-deadline:
			t.Fatal("the loop did not survive a panicking pass")
		case <-time.After(5 * time.Millisecond):
		}
	}
	e.svc.StopWakeBackstop()
}

func TestBackstop_LifecycleStartStopLatch(t *testing.T) {
	e := newWakeEnv(t)
	var passes atomic.Int32
	e.svc.SetBackstopHooks(Hooks{TurnDuties: func(context.Context, string) error {
		passes.Add(1)
		return nil
	}})
	e.svc.backstop.interval = 40 * time.Millisecond

	started := time.Now()
	e.svc.StartWakeBackstop(context.Background())
	e.svc.StartWakeBackstop(context.Background())
	for passes.Load() == 0 {
		if time.Since(started) > 5*time.Second {
			t.Fatal("no pass ran")
		}
		time.Sleep(time.Millisecond)
	}
	if elapsed := time.Since(started); elapsed < 40*time.Millisecond {
		t.Fatalf("first pass after %v, before one interval", elapsed)
	}
	e.svc.StopWakeBackstop()
	e.svc.StopWakeBackstop()
	stopped := passes.Load()

	e.svc.StartWakeBackstop(context.Background())
	time.Sleep(120 * time.Millisecond)
	if passes.Load() != stopped {
		t.Fatal("Start after Stop must be a no-op")
	}
}

func TestBackstop_StopBeforeStartLatchesClosed(t *testing.T) {
	e := newWakeEnv(t)
	var passes atomic.Int32
	e.svc.SetBackstopHooks(Hooks{TurnDuties: func(context.Context, string) error {
		passes.Add(1)
		return nil
	}})
	e.svc.backstop.interval = 10 * time.Millisecond
	e.svc.StopWakeBackstop()
	e.svc.StartWakeBackstop(context.Background())
	time.Sleep(60 * time.Millisecond)
	if passes.Load() != 0 {
		t.Fatal("a backstop stopped before Start must never run")
	}
}

func TestBackstop_StopWaitsForAPassInProgress(t *testing.T) {
	e := newWakeEnv(t)
	inDuty := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	e.svc.SetBackstopHooks(Hooks{TurnDuties: func(context.Context, string) error {
		once.Do(func() { close(inDuty) })
		<-release
		return nil
	}})
	e.svc.backstop.interval = 5 * time.Millisecond
	e.svc.StartWakeBackstop(context.Background())
	<-inDuty
	stopped := make(chan struct{})
	go func() { defer close(stopped); e.svc.StopWakeBackstop() }()
	select {
	case <-stopped:
		t.Fatal("Stop returned while a pass was running")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	<-stopped
}

func TestBackstop_CapFillsInTaskThenKindOrder(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-a")
	e.ownTask(t, "t-b")
	for _, id := range []string{"t-a", "t-b"} {
		e.sources.state[id] = "COMPLETED"
		e.sources.errStamp["s-"+id] = "e-" + id
	}
	seedPendingWakes(t, e.fix, pendingWakeCap-1)
	e.pass(t)
	got := e.wakes(t)
	var mine []storedWake
	for _, w := range got {
		if w.TaskID == "t-a" || w.TaskID == "t-b" {
			mine = append(mine, w)
		}
	}
	if len(mine) != 1 || mine[0].TaskID != "t-a" || mine[0].Kind != "error" {
		t.Fatalf("one free slot must go to the first task's first kind, got %+v", mine)
	}
}

func TestBackstop_StallCurrency(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	detected := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	mustExec(t, e.store, `INSERT INTO coordinator_stalls (task_id, workspace_id, detected_at, last_event_at, stalled_for_ms) VALUES ('t-1', 'ws-1', ?, ?, 7200000)`, detected, detected.Add(-2*time.Hour))
	after := detected.Add(time.Minute)
	e.sources.last["t-1"] = &after
	e.pass(t)
	wantWakes(t, e)
	e.sources.last["t-1"] = nil
	e.pass(t)
	if len(e.wakes(t)) != 1 {
		t.Fatalf("an absent LastActivityAt keeps the stall current, got %+v", e.wakes(t))
	}
}

func TestBackstop_ArchivePruneUnarchiveRecordsNoSecondWake(t *testing.T) {
	e := newWakeEnv(t)
	e.ownTask(t, "t-1")
	e.sources.state["t-1"] = "COMPLETED"
	e.pass(t)
	wantWakes(t, e, storedWake{"t-1", "completed", "completed", "pending"})
	mustExec(t, e.store, `UPDATE coordinator_wakes SET status = 'delivered'`)

	mustExec(t, e.store, `UPDATE tasks SET archived_at = ? WHERE id = 't-1'`, time.Now().UTC())
	e.pass(t)
	wantWakes(t, e, storedWake{"t-1", "completed", "completed", "delivered"})

	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	mustExec(t, e.store, `UPDATE coordinator_wakes SET updated_at = ?`, old)
	if _, _, err := e.store.PruneWakeState(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	wantWakes(t, e, storedWake{"t-1", "completed", "completed", "delivered"})

	mustExec(t, e.store, `UPDATE tasks SET archived_at = NULL WHERE id = 't-1'`)
	e.pass(t)
	wantWakes(t, e, storedWake{"t-1", "completed", "completed", "delivered"})
}

func TestBackstop_NilWakeSourcesStillDelivers(t *testing.T) {
	e := newWakeEnv(t)
	log := &callLog{}
	e.svc.SetBackstopHooks(Hooks{Deliver: log.duty("deliver")})
	e.svc.backstop.runPass(context.Background())
	if !equalStrings(log.got(), []string{"deliver:" + e.fix.c.ID}) {
		t.Fatalf("calls = %v", log.got())
	}
}
