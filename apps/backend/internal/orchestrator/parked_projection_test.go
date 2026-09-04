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
}

func newParkedTestRepo(sessions ...*models.TaskSession) *parkedTestRepo {
	r := &parkedTestRepo{sessions: make(map[string]*models.TaskSession)}
	for _, s := range sessions {
		r.sessions[s.ID] = s
	}
	return r
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
	bus := &recordingEventBus{}
	probe := &spyBackgroundProbe{}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	svc := &Service{
		logger:           testLogger(),
		eventBus:         bus,
		repo:             repo,
		parkedStates:     make(map[string]*parkedSessionState),
		parkedEpoch:      1,
		parkedLoopCtx:    loopCtx,
		parkedLoopCancel: loopCancel,
	}
	svc.SetBackgroundProbe(probe)
	t.Cleanup(svc.stopParkedSamplingLoops)
	return svc, bus, probe
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
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
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
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
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
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
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
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultUnknown}
	probe.errs = []error{context.DeadlineExceeded}
	svc.setObservedDetachedLaunch("sess-1")

	svc.settleParkedProjectionSync(context.Background(), "task-1", "sess-1")

	if parked, _ := svc.ParkedSnapshot("sess-1"); parked {
		t.Fatal("expected not parked when the probe errors")
	}
}

// AC-68/D8: leaving WAITING_FOR_INPUT clears the projection immediately
// without a new probe sample; last_sample is never re-read for this
// transition.
func TestClearParkedOnSessionStateLeft_ClearsWithoutNewSample(t *testing.T) {
	repo := newParkedTestRepo(&models.TaskSession{ID: "sess-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
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
