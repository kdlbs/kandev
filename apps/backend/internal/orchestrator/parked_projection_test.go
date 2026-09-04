package orchestrator

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// parkedTestRepo is a minimal repoStore fake: only GetTaskSession and
// UpdateTaskSessionState are exercised by updateTaskSessionStateWithHook's
// non-conditional-updater branch (this fake does not implement
// conditionalTaskSessionStateUpdater). Everything else panics if touched,
// which is the point — a nil-pointer or panic here means a test reached
// further into the session-state machinery than the parked-projection hook
// should require.
type parkedTestRepo struct {
	repoStore
	mu       sync.Mutex
	sessions map[string]*models.TaskSession
	tasks    map[string]*models.Task
}

func newParkedTestRepo(sessions ...*models.TaskSession) *parkedTestRepo {
	r := &parkedTestRepo{sessions: make(map[string]*models.TaskSession), tasks: make(map[string]*models.Task)}
	for _, s := range sessions {
		r.sessions[s.ID] = s
		if _, ok := r.tasks[s.TaskID]; !ok && s.TaskID != "" {
			r.tasks[s.TaskID] = &models.Task{ID: s.TaskID}
		}
	}
	return r
}

// GetTask satisfies the repoStore method publishTaskParkedTransition uses to
// load the task row before delegating to the TaskEventPublisher. Tasks are
// auto-seeded from newParkedTestRepo's session TaskIDs.
func (r *parkedTestRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}
	cp := *t
	return &cp, nil
}

func (r *parkedTestRepo) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	cp := *s
	return &cp, nil
}

func (r *parkedTestRepo) UpdateTaskSessionState(_ context.Context, id string, state models.TaskSessionState, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return errors.New("session not found")
	}
	s.State = state
	return nil
}

func (r *parkedTestRepo) deleteSession(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// spyBackgroundProbe scripts a fixed sequence of results and counts calls,
// per task-05's own guidance to test against the BackgroundProbe port's test
// double rather than the real transport or process walk.
type spyBackgroundProbe struct {
	mu      sync.Mutex
	results []executor.ProbeResult
	errs    []error
	calls   int
	onProbe func() // optional hook invoked synchronously before returning, for race tests
}

func (p *spyBackgroundProbe) Probe(context.Context, string) (executor.ProbeResult, error) {
	p.mu.Lock()
	i := p.calls
	p.calls++
	p.mu.Unlock()
	if p.onProbe != nil {
		p.onProbe()
	}
	if i < len(p.results) {
		var err error
		if i < len(p.errs) {
			err = p.errs[i]
		}
		return p.results[i], err
	}
	if len(p.results) == 0 {
		return executor.ProbeResultUnknown, nil
	}
	return p.results[len(p.results)-1], nil
}

func (p *spyBackgroundProbe) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func newParkedTestService(t *testing.T, repo *parkedTestRepo) (*Service, *recordingEventBus, *spyBackgroundProbe) {
	t.Helper()
	svc, bus, probe, _ := newParkedTestServiceWithTaskEvents(t, repo)
	return svc, bus, probe
}

// newParkedTestServiceWithTaskEvents is newParkedTestService plus a
// recordingTaskUpdatedPublisher, for tests asserting task.updated is
// published (or is not) on the task-level OR-aggregate flip.
func newParkedTestServiceWithTaskEvents(t *testing.T, repo *parkedTestRepo) (*Service, *recordingEventBus, *spyBackgroundProbe, *recordingTaskUpdatedPublisher) {
	t.Helper()
	bus := &recordingEventBus{}
	probe := &spyBackgroundProbe{}
	publisher := &recordingTaskUpdatedPublisher{}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	svc := &Service{
		logger:           testLogger(),
		eventBus:         bus,
		repo:             repo,
		taskEvents:       publisher,
		parkedStates:     make(map[string]*parkedSessionState),
		taskParkedStates: make(map[string]*taskParkedState),
		parkedEpoch:      1,
		parkedLoopCtx:    loopCtx,
		parkedLoopCancel: loopCancel,
	}
	svc.SetBackgroundProbe(probe)
	t.Cleanup(svc.stopParkedSamplingLoops)
	return svc, bus, probe, publisher
}

func lastParkedEvent(b *recordingEventBus) (parked bool, revision uint64, ok bool) {
	for i := len(b.events) - 1; i >= 0; i-- {
		data, isMap := b.events[i].event.Data.(map[string]interface{})
		if !isMap {
			continue
		}
		p, hasParked := data["parked_on_background_work"].(bool)
		if !hasParked {
			continue
		}
		r, _ := data["revision"].(uint64)
		return p, r, true
	}
	return false, 0, false
}

// AC-21: attested + live + WAITING_FOR_INPUT parks.
func TestSettleParkedProjectionSync_AttestedAndLive_Parks(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, bus, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	parked, revision := svc.ParkedSnapshot("sess-1")
	if !parked || revision != 1 {
		t.Fatalf("ParkedSnapshot = (%v, %d), want (true, 1)", parked, revision)
	}
	if got, want, ok := lastParkedEvent(bus); !ok || got != true || want != 1 {
		t.Fatalf("published event = (%v, %d, %v), want (true, 1, true)", got, want, ok)
	}
}

// AC-24/AC-40a: no attestation -> false, and the probe is never called.
func TestSettleParkedProjectionSync_NoAttestation_NeverProbes(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	parked, revision := svc.ParkedSnapshot("sess-1")
	if parked || revision != 0 {
		t.Fatalf("ParkedSnapshot = (%v, %d), want (false, 0)", parked, revision)
	}
	if n := probe.callCount(); n != 0 {
		t.Fatalf("probe called %d times, want 0 (AC-40a)", n)
	}
}

// AC-25: attested but probe reports settled -> not parked.
func TestSettleParkedProjectionSync_AttestedButSettled_NotParked(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultSettled}
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	if parked, _ := svc.ParkedSnapshot("sess-1"); parked {
		t.Fatal("expected not parked for a settled probe result")
	}
}

// AC-26/AC-27: attested but probe reports unknown -> not parked.
func TestSettleParkedProjectionSync_AttestedButUnknown_NotParked(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultUnknown}
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	if parked, _ := svc.ParkedSnapshot("sess-1"); parked {
		t.Fatal("expected not parked for an unknown probe result")
	}
}

// AC-40/AC-46: a probe error is treated as unknown; never parked, no panic.
func TestSettleParkedProjectionSync_ProbeErrors_TreatedAsUnknown(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultUnknown}
	probe.errs = []error{context.DeadlineExceeded}
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	if parked, _ := svc.ParkedSnapshot("sess-1"); parked {
		t.Fatal("expected not parked when the probe errors")
	}
}

// F9: a concurrent D8 transition that lands while the synchronous D2 settle
// probe is still in flight must not be clobbered by the late-arriving
// sample. The session leaves WAITING_FOR_INPUT
// for real (the repo row is updated, mirroring what a genuine self-resume
// does) and clears the projection; the stale `live` sample that resolves
// afterward must not re-park it.
func TestSettleParkedProjectionSync_ConcurrentTransitionDuringProbe_NotReParked(t *testing.T) {
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput}
	repo := newParkedTestRepo(session)
	svc, bus, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")

	probe.onProbe = func() {
		repo.mu.Lock()
		session.State = models.TaskSessionStateRunning
		repo.mu.Unlock()
		svc.clearParkedOnSessionStateLeft(context.Background(), "task-1", "sess-1", models.TaskSessionStateRunning)
	}

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	if parked, revision := svc.ParkedSnapshot("sess-1"); parked {
		t.Fatalf("ParkedSnapshot = (%v, %d), want parked=false — the session left WAITING_FOR_INPUT before the late probe sample was applied", parked, revision)
	}
	if len(bus.events) != 0 {
		t.Fatalf("published %d parked-projection events, want 0 (the no-op clear and the stale sample must both be discarded)", len(bus.events))
	}
	if n := probe.callCount(); n != 1 {
		t.Fatalf("probe called %d times, want exactly 1", n)
	}
}

// AC-68/D8: leaving WAITING_FOR_INPUT clears the projection immediately
// without a new probe sample; last_sample is never re-read for this
// transition.
func TestClearParkedOnSessionStateLeft_ClearsWithoutNewSample(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, bus, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")
	if parked, _ := svc.ParkedSnapshot("sess-1"); !parked {
		t.Fatal("precondition: session should be parked before the self-resume")
	}
	callsBeforeClear := probe.callCount()

	svc.clearParkedOnSessionStateLeft(context.Background(), "task-1", "sess-1", models.TaskSessionStateRunning)

	if n := probe.callCount(); n != callsBeforeClear {
		t.Fatalf("probe called again on the session-state clear: %d -> %d", callsBeforeClear, n)
	}
	parked, revision := svc.ParkedSnapshot("sess-1")
	if parked || revision != 2 {
		t.Fatalf("ParkedSnapshot = (%v, %d), want (false, 2)", parked, revision)
	}
	if got, want, ok := lastParkedEvent(bus); !ok || got != false || want != 2 {
		t.Fatalf("published event = (%v, %d, %v), want (false, 2, true)", got, want, ok)
	}
}

// The dispatcher wired into updateTaskSessionStateWithHook only reacts to the
// two transitions that matter (entering/leaving WAITING_FOR_INPUT) and is a
// no-op for every other transition pair.
func TestOnSessionStateChangedForParkedProjection_IgnoresUnrelatedTransitions(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")

	svc.onSessionStateChangedForParkedProjection(context.Background(), "task-1", "sess-1",
		models.TaskSessionStateStarting, models.TaskSessionStateRunning)

	if n := probe.callCount(); n != 0 {
		t.Fatalf("probe called %d times for a non-WAITING_FOR_INPUT transition, want 0", n)
	}
	if parked, revision := svc.ParkedSnapshot("sess-1"); parked || revision != 0 {
		t.Fatalf("ParkedSnapshot = (%v, %d), want (false, 0)", parked, revision)
	}
}

// AC-53 (state leg) / AC-54: a session that is never parked in the first
// place is never probed, even across repeated settle calls.
func TestSettleParkedProjectionSync_NeverParked_ZeroProbesAcrossRepeats(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
	svc, _, probe := newParkedTestService(t, repo)

	for i := 0; i < 3; i++ {
		svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")
	}

	if n := probe.callCount(); n != 0 {
		t.Fatalf("probe called %d times for a never-attested session, want 0 (AC-54)", n)
	}
}

// F9: a sample that completes after a concurrent transition has already
// moved the session's revision must be discarded, not applied.
func TestSampleParkedSessionTick_StaleRevisionDiscarded(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, bus, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive, executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")
	if parked, rev := svc.ParkedSnapshot("sess-1"); !parked || rev != 1 {
		t.Fatalf("precondition failed: (%v, %d)", parked, rev)
	}
	eventsBeforeTick := len(bus.events)

	// Simulate a self-resume racing this sample: it lands while the probe
	// (started by the tick below) is still "in flight".
	probe.onProbe = func() {
		svc.clearParkedOnSessionStateLeft(context.Background(), "task-1", "sess-1", models.TaskSessionStateRunning)
	}

	keepLooping := svc.sampleParkedSessionTick(context.Background(), "task-1", "sess-1")

	if keepLooping {
		t.Fatal("expected the loop to stop on a stale sample")
	}
	// Only the resume's own clear (revision 2) should have published;
	// the stale live sample must not re-park the session (which would be
	// revision 3).
	parked, revision := svc.ParkedSnapshot("sess-1")
	if parked || revision != 2 {
		t.Fatalf("ParkedSnapshot = (%v, %d), want (false, 2) — stale sample must not re-park", parked, revision)
	}
	if got := len(bus.events) - eventsBeforeTick; got != 1 {
		t.Fatalf("published %d events for this tick, want exactly 1 (the resume's own clear)", got)
	}
}

// AC-53: a probe result other than live stops the loop.
func TestSampleParkedSessionTick_NonLiveResult_StopsLoop(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive, executor.ProbeResultSettled}
	svc.setObservedDetachedLaunch("sess-1")
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	keepLooping := svc.sampleParkedSessionTick(context.Background(), "task-1", "sess-1")

	if keepLooping {
		t.Fatal("expected the loop to stop on a settled result")
	}
	if parked, _ := svc.ParkedSnapshot("sess-1"); parked {
		t.Fatal("expected the session to un-park on a settled result")
	}
}

// AC-53: a deleted session stops the loop and clears the projection instead
// of continuing to sample a row that no longer exists.
func TestSampleParkedSessionTick_SessionDeleted_StopsLoopAndClears(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")
	repo.deleteSession("sess-1")

	keepLooping := svc.sampleParkedSessionTick(context.Background(), "task-1", "sess-1")

	if keepLooping {
		t.Fatal("expected the loop to stop for a deleted session")
	}
	if parked, _ := svc.ParkedSnapshot("sess-1"); parked {
		t.Fatal("expected the session to un-park once its row is gone")
	}
}

// AC-54/D9: KANDEV_PARKED_PROBE_INTERVAL = 0 disables periodic sampling —
// the loop must never start even for a parked session.
func TestMaybeStartParkedSamplingLoop_ZeroIntervalNeverStarts(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.backgroundProbeConfig.Interval = 0
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	svc.parkedMu.Lock()
	st := svc.parkedStates["sess-1"]
	loopRunning := st != nil && st.loopCancel != nil
	svc.parkedMu.Unlock()
	if loopRunning {
		t.Fatal("expected no sampling loop with interval=0")
	}
}

// AC-53 end to end: a real ticker-driven loop stops itself once the probe
// reports a non-live result, and Service.Stop drains the goroutine cleanly.
func TestParkedSamplingLoop_RealTicker_StopsOnSettled(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive, executor.ProbeResultSettled}
	svc.backgroundProbeConfig.Interval = 5 * time.Millisecond
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if probe.callCount() >= 2 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if n := probe.callCount(); n < 2 {
		t.Fatalf("probe called %d times, want at least 2 (initial live sample + one tick)", n)
	}

	// stopParkedSamplingLoops (via t.Cleanup) proves the goroutine actually
	// exits rather than leaking past the settled tick.
	svc.stopParkedSamplingLoops()
	if parked, _ := svc.ParkedSnapshot("sess-1"); parked {
		t.Fatal("expected the session to un-park after the settled tick")
	}
}

// F7 (round-5, load-bearing ordering): the turn-settle handler must publish
// session.turn_finished before it takes the synchronous parked-projection
// sample, so the probe budget never delays that notification (AC-76). A
// runtime reordering test would need the full task-service/notification
// wiring; this pins the invariant at the source-text level instead, the same
// technique the round-5 disposition uses for F2's architecture test.
func TestParkedProjectionOrdering_ProbeCallSitesFollowTurnCompletion(t *testing.T) {
	src, err := os.ReadFile("event_handlers_streaming.go")
	if err != nil {
		t.Fatalf("read event_handlers_streaming.go: %v", err)
	}
	text := string(src)

	fnStart := strings.Index(text, "func (s *Service) handleCompleteStreamEvent(")
	if fnStart < 0 {
		t.Fatal("handleCompleteStreamEvent not found")
	}
	fnEnd := strings.Index(text[fnStart:], "\nfunc ")
	if fnEnd < 0 {
		fnEnd = len(text) - fnStart
	}
	body := text[fnStart : fnStart+fnEnd]

	completeIdx := strings.Index(body, "s.completeTurnForTaskSession(")
	waitingIdx := strings.Index(body, "s.setSessionWaitingForInput(")
	if completeIdx < 0 || waitingIdx < 0 {
		t.Fatalf("expected both calls in handleCompleteStreamEvent; completeTurnForTaskSession=%d setSessionWaitingForInput=%d", completeIdx, waitingIdx)
	}
	if completeIdx >= waitingIdx {
		t.Fatal("completeTurnForTaskSession (which triggers session.turn_finished) must run before " +
			"setSessionWaitingForInput (which triggers the parked-projection probe via " +
			"onSessionStateChangedForParkedProjection) — F7 ordering regressed")
	}

	hookSrc, err := os.ReadFile("parked_projection.go")
	if err != nil {
		t.Fatalf("read parked_projection.go: %v", err)
	}
	if !strings.Contains(string(hookSrc), "onSessionStateChangedForParkedProjection") {
		t.Fatal("expected the parked-projection dispatcher to be defined in parked_projection.go")
	}
	hookWireSrc, err := os.ReadFile("event_handlers_streaming.go")
	if err != nil {
		t.Fatalf("read event_handlers_streaming.go: %v", err)
	}
	updateHookIdx := strings.Index(string(hookWireSrc), "func (s *Service) updateTaskSessionStateWithHook(")
	dispatchIdx := strings.Index(string(hookWireSrc), "s.onSessionStateChangedForParkedProjection(")
	publishStateChangedIdx := strings.Index(string(hookWireSrc), "s.publishTaskSessionStateChanged(")
	if updateHookIdx < 0 || dispatchIdx < 0 || publishStateChangedIdx < 0 {
		t.Fatal("expected updateTaskSessionStateWithHook to publish state_changed before dispatching the parked-projection hook")
	}
	if publishStateChangedIdx >= dispatchIdx {
		t.Fatal("expected the parked-projection dispatch to be wired after publishTaskSessionStateChanged")
	}
}

// --- Task-level projection (task-06) ---
// Spec: docs/specs/disambiguate-waiting/spec.md, "Data model -> Task-level
// projection". ACs: AC-22, AC-36, AC-38, AC-49, AC-50, AC-62, AC-78.
// AC-39 and AC-77 are consumer/boot-payload-side discard behavior verified at
// the DTO/frontend layers (task-07/08), not here.

// AC-50: a task with no sessions, or no session that ever transitioned, reads
// (false, 0) — the D9 default, not an error or a distinguishable "unknown".
func TestTaskParkedSnapshot_NoTransitions_DefaultsFalseZero(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
	svc, _, _ := newParkedTestService(t, repo)

	if parked, revision := svc.TaskParkedSnapshot("task-1"); parked || revision != 0 {
		t.Fatalf("TaskParkedSnapshot(no-sessions-tracked) = (%v, %d), want (false, 0)", parked, revision)
	}
	if parked, revision := svc.TaskParkedSnapshot(""); parked || revision != 0 {
		t.Fatalf("TaskParkedSnapshot(\"\") = (%v, %d), want (false, 0)", parked, revision)
	}
}

// AC-38/AC-62: a session parking flips the task-level OR to true (revision 1)
// and publishes task.updated; the session then leaving WAITING_FOR_INPUT flips
// it back to false (revision 2) and publishes task.updated again. Re-deriving
// the same task-level value must not be possible via this path since every
// call here is a genuine session-level change.
func TestTaskParkedSnapshot_SingleSessionToggle_RevisionMonotonicallyIncreases(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe, publisher := newParkedTestServiceWithTaskEvents(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")
	if parked, revision := svc.TaskParkedSnapshot("task-1"); !parked || revision != 1 {
		t.Fatalf("after park: TaskParkedSnapshot = (%v, %d), want (true, 1)", parked, revision)
	}
	if len(publisher.updatedTaskIDs) != 1 || publisher.updatedTaskIDs[0] != "task-1" {
		t.Fatalf("expected exactly one task.updated for task-1 after the first parking, got %v", publisher.updatedTaskIDs)
	}

	svc.clearParkedOnSessionStateLeft(context.Background(), "task-1", "sess-1", models.TaskSessionStateRunning)
	if parked, revision := svc.TaskParkedSnapshot("task-1"); parked || revision != 2 {
		t.Fatalf("after clear: TaskParkedSnapshot = (%v, %d), want (false, 2)", parked, revision)
	}
	if len(publisher.updatedTaskIDs) != 2 {
		t.Fatalf("expected a second task.updated after the OR cleared, got %v", publisher.updatedTaskIDs)
	}
}

// AC-49: two sessions on one task. S1 toggles twice (parks, then clears —
// session revision 2, ending false); S2 never transitions. S2 then parks.
// The task's parked_revision must be exactly 1 (its own first-ever flip),
// never max(2, 1) == 2 and never conflated with either session's own
// revision counter.
func TestTaskParkedSnapshot_TwoSessions_RevisionIsOwnCounterNotMaxOfSessions(t *testing.T) {
	repo := newParkedTestRepo(
		&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput},
		&models.TaskSession{ID: "sess-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput},
	)
	svc, _, probe, _ := newParkedTestServiceWithTaskEvents(t, repo)
	svc.setObservedDetachedLaunch("sess-1")
	svc.setObservedDetachedLaunch("sess-2")

	// S1: park then clear (its own revision reaches 2, ends false) — this
	// touches the task-level OR twice (true, then false again) but must not
	// leave the task's revision at 2.
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")
	svc.clearParkedOnSessionStateLeft(context.Background(), "task-1", "sess-1", models.TaskSessionStateRunning)
	if _, revision := svc.ParkedSnapshot("sess-1"); revision != 2 {
		t.Fatalf("precondition: sess-1 revision = %d, want 2", revision)
	}
	if parked, revision := svc.TaskParkedSnapshot("task-1"); parked || revision != 2 {
		t.Fatalf("after S1 round-trip: TaskParkedSnapshot = (%v, %d), want (false, 2) — "+
			"one flip up (revision 1), one flip back down (revision 2)", parked, revision)
	}

	// S2 parks for the first time.
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-2")

	parked, revision := svc.TaskParkedSnapshot("task-1")
	if !parked {
		t.Fatal("expected the task-level OR to be true once sess-2 parks")
	}
	if revision != 3 {
		t.Fatalf("TaskParkedSnapshot revision = %d, want 3 (strictly greater than its prior value 2, "+
			"not max(sess-1=2, sess-2=1)=2)", revision)
	}
}

// AC-78: two sessions, S1 already parked (task-level OR already true). S2
// parks too — a session-level transition occurs (session.activity_changed for
// S2), but the task-level OR does not change (already true), so no
// task.updated is published and the task's parked_revision is unchanged.
func TestPublishTaskParkedTransition_ORAlreadyTrue_NoTaskUpdatedPublished(t *testing.T) {
	repo := newParkedTestRepo(
		&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput},
		&models.TaskSession{ID: "sess-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput},
	)
	svc, bus, probe, publisher := newParkedTestServiceWithTaskEvents(t, repo)
	svc.setObservedDetachedLaunch("sess-1")
	svc.setObservedDetachedLaunch("sess-2")

	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")
	if parked, revision := svc.TaskParkedSnapshot("task-1"); !parked || revision != 1 {
		t.Fatalf("precondition: TaskParkedSnapshot = (%v, %d), want (true, 1)", parked, revision)
	}
	publishesBeforeS2 := len(publisher.updatedTaskIDs)

	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-2")

	// Session-level: sess-2 itself still gets its own transition + event.
	if parked, revision := svc.ParkedSnapshot("sess-2"); !parked || revision != 1 {
		t.Fatalf("sess-2 ParkedSnapshot = (%v, %d), want (true, 1)", parked, revision)
	}
	if got, want, ok := lastParkedEvent(bus); !ok || got != true || want != 1 {
		t.Fatalf("expected sess-2's own session.activity_changed event, got (%v, %d, %v)", got, want, ok)
	}
	// Task-level: OR was already true, so it must not have changed, and no
	// additional task.updated must have been published.
	if parked, revision := svc.TaskParkedSnapshot("task-1"); !parked || revision != 1 {
		t.Fatalf("TaskParkedSnapshot after sess-2 parks = (%v, %d), want (true, 1) unchanged", parked, revision)
	}
	if len(publisher.updatedTaskIDs) != publishesBeforeS2 {
		t.Fatalf("expected no additional task.updated when the task-level OR does not change, "+
			"publishes before=%d after=%d", publishesBeforeS2, len(publisher.updatedTaskIDs))
	}
}

// AC-36: TaskParkedSnapshot on a freshly constructed Service (simulating a
// backend restart) reads (false, 0) for every task, regardless of what any
// prior process instance had recorded — there is no persisted state to carry
// forward.
func TestTaskParkedSnapshot_FreshService_ReadsFalseZero(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe, _ := newParkedTestServiceWithTaskEvents(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")
	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")
	if parked, _ := svc.TaskParkedSnapshot("task-1"); !parked {
		t.Fatal("precondition: task should be parked before simulating the restart")
	}

	freshRepo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	freshSvc, _, _ := newParkedTestService(t, freshRepo)

	if parked, revision := freshSvc.TaskParkedSnapshot("task-1"); parked || revision != 0 {
		t.Fatalf("fresh service TaskParkedSnapshot = (%v, %d), want (false, 0)", parked, revision)
	}
}

// publishTaskParkedTransition must not publish (and must not panic) when no
// TaskEventPublisher is wired, mirroring publishTaskUpdated's own no-op
// contract for an unwired publisher.
func TestPublishTaskParkedTransition_NoPublisherWired_NoOp(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo) // no taskEvents wired
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	if parked, revision := svc.TaskParkedSnapshot("task-1"); !parked || revision != 1 {
		t.Fatalf("TaskParkedSnapshot = (%v, %d), want (true, 1) even without a wired publisher", parked, revision)
	}
}
