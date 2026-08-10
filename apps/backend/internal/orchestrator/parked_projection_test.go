package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// fakeBackgroundProbe is a test double for backgroundProbePort that returns a
// fixed result and counts invocations.
type fakeBackgroundProbe struct {
	mu     sync.Mutex
	result string
	err    error
	calls  int
}

func (f *fakeBackgroundProbe) ProbeBackgroundWorkloads(_ context.Context, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.result, f.err
}

func (f *fakeBackgroundProbe) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// parkedTestService builds a Service wired for the parked-projection tests:
// a real sqlite repo (so currentSessionState reads real persisted state),
// the flag forced on, and a fake probe. Takes the backgroundProbePort
// interface rather than *fakeBackgroundProbe so tests can also supply other
// doubles (e.g. blockingProbe) without widening fakeBackgroundProbe itself.
func parkedTestService(t *testing.T, taskID, sessionID string, probe backgroundProbePort) *Service {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	svc.config.ParkedOnBackgroundWork = true
	svc.SetBackgroundProbe(probe)
	return svc
}

func attestShellLaunch(ctx context.Context, svc *Service, taskID, sessionID string) {
	svc.handleToolCallEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			ToolCallID: "tc-shell-1",
			ToolStatus: "running",
			Normalized: attestedBackgroundShellPayload("sleep 300 &"),
		},
	})
}

// TestParked_FormulaRequiresAllThreeTerms covers AC-21/AC-22/AC-24/AC-25/AC-26:
// the three-term AND formula (observedDetached && lastSample=="live" &&
// state==WAITING_FOR_INPUT).
func TestParked_FormulaRequiresAllThreeTerms(t *testing.T) {
	ctx := context.Background()

	t.Run("attested+live -> parked true, task true, foreground_activity untouched (AC-21/AC-22)", func(t *testing.T) {
		probe := &fakeBackgroundProbe{result: probeResultLive}
		svc := parkedTestService(t, "t1", "s1", probe)
		attestShellLaunch(ctx, svc, "t1", "s1")
		beforeActivity := svc.ForegroundActivity("s1")

		svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

		require.True(t, svc.ParkedProjectionSnapshot("s1"), "session should be parked")
		require.True(t, svc.TaskParkedProjectionSnapshot("t1"), "task should be parked")
		require.Equal(t, 1, probe.callCount())
		// AC-22: parking is a projection layered on top of, not a mutation of,
		// foreground_activity — the settle hook must not change what the
		// existing foreground-activity computation reports. Previously this
		// clause was named in the subtest but never asserted (Review round 2
		// should-fix item 1).
		require.Equal(t, beforeActivity, svc.ForegroundActivity("s1"),
			"parking must not change foreground_activity from what it reported before the settle hook ran")
	})

	t.Run("no attestation -> false regardless of probe result (AC-24)", func(t *testing.T) {
		probe := &fakeBackgroundProbe{result: probeResultLive}
		svc := parkedTestService(t, "t1", "s1", probe)
		// No attestShellLaunch call.

		svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

		require.False(t, svc.ParkedProjectionSnapshot("s1"))
		require.Equal(t, 0, probe.callCount(), "AC-40a: no attestation means zero probes")
	})

	t.Run("attested+settled -> false (AC-25)", func(t *testing.T) {
		probe := &fakeBackgroundProbe{result: probeResultSettled}
		svc := parkedTestService(t, "t1", "s1", probe)
		attestShellLaunch(ctx, svc, "t1", "s1")

		svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

		require.False(t, svc.ParkedProjectionSnapshot("s1"))
	})

	t.Run("attested+unknown -> false (AC-26)", func(t *testing.T) {
		probe := &fakeBackgroundProbe{result: probeResultUnknown}
		svc := parkedTestService(t, "t1", "s1", probe)
		attestShellLaunch(ctx, svc, "t1", "s1")

		svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

		require.False(t, svc.ParkedProjectionSnapshot("s1"))
	})
}

// TestParked_FlagOff_NoProbeIssued verifies V1-09: with the flag off, zero
// probes are issued and the session never parks, even with an attestation
// that would otherwise satisfy the formula.
func TestParked_FlagOff_NoProbeIssued(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)
	svc.config.ParkedOnBackgroundWork = false
	attestShellLaunch(ctx, svc, "t1", "s1")

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

	require.False(t, svc.ParkedProjectionSnapshot("s1"))
	require.Equal(t, 0, probe.callCount())
}

// TestParked_NoRecogniserMatch verifies AC-37: a background tool call whose
// kind is NOT shell (here: a subagent, built via attestedDetachedSubagentPayload
// with Detached=true — matching the real stampSubagentBackgroundWork producer
// AC-37's second GIVEN names) never attests, so a later settle issues zero
// probes and never parks. The IsDetachedBackgroundLaunch precondition below
// makes sure this test actually exercises the Kind==shell filter: without
// Detached=true, IsDetachedBackgroundLaunch is already false and the
// assertions below would hold regardless of whether the filter exists.
func TestParked_NoRecogniserMatch(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)

	payload := attestedDetachedSubagentPayload("desc", "prompt", "general")
	require.True(t, payload.IsDetachedBackgroundLaunch(),
		"precondition: payload must be ambiguous to IsDetachedBackgroundLaunch so the Kind==shell filter is what's under test")

	svc.handleToolCallEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			ToolCallID: "tc-subagent-1",
			ToolStatus: "running",
			Normalized: payload,
		},
	})

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

	require.False(t, svc.ParkedProjectionSnapshot("s1"))
	require.Equal(t, 0, probe.callCount())
}

// TestParked_ClearOnRunning verifies V1-01 and AC-68: an observed transition
// into RUNNING clears the attestation, so a subsequent settle with no new
// attested launch computes false and issues zero probes; and a session
// parked at the moment it enters RUNNING un-parks immediately with no
// further sample taken.
func TestParked_ClearOnRunning(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)
	attestShellLaunch(ctx, svc, "t1", "s1")
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)
	require.True(t, svc.ParkedProjectionSnapshot("s1"), "precondition: session should be parked")
	require.Equal(t, 1, probe.callCount())

	// Operator submits a prompt; session transitions WAITING_FOR_INPUT -> RUNNING.
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateRunning, "", true)

	require.False(t, svc.ParkedProjectionSnapshot("s1"), "AC-68: parked clears on entering RUNNING")
	require.False(t, svc.TaskParkedProjectionSnapshot("t1"))
	require.Equal(t, 1, probe.callCount(), "AC-68: no further sample is taken")

	// A later settle with no new attestation issues zero probes (V1-01).
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)
	require.Equal(t, 1, probe.callCount(), "V1-01: attestation was cleared, so this settle probes nothing")
	require.False(t, svc.ParkedProjectionSnapshot("s1"))
}

// TestParked_ClearOnStarting verifies V1-02: an observed transition into
// STARTING clears the attestation, so a later STARTING -> WAITING_FOR_INPUT
// heal issues zero probes and reports false.
func TestParked_ClearOnStarting(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)
	attestShellLaunch(ctx, svc, "t1", "s1")

	// Simulate the backend observing the session re-enter STARTING (e.g. the
	// stale-STARTING heal or ResetAgentContext restore) before it settles.
	svc.handleParkedStateTransition(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateStarting)

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

	require.Equal(t, 0, probe.callCount(), "V1-02: attestation was cleared on entering STARTING")
	require.False(t, svc.ParkedProjectionSnapshot("s1"))
}

// TestParked_V1_03_DiscardsRaceWithConcurrentTransition verifies §7.2's
// revalidation rule: a probe sample that completes after the session's
// observed-transition state changed underneath it (here: the session enters
// RUNNING WHILE the probe is "in flight", simulated inside the fake probe's
// call) is discarded — nothing is written, matching V1-03.
func TestParked_V1_03_DiscardsRaceWithConcurrentTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	svc.config.ParkedOnBackgroundWork = true

	probe := &racingProbe{svc: svc, taskID: "t1", sessionID: "s1", result: probeResultLive}
	svc.SetBackgroundProbe(probe)
	attestShellLaunch(ctx, svc, "t1", "s1")

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

	ps := svc.parkedStateFor("s1")
	require.NotNil(t, ps)
	require.Empty(t, ps.lastSample, "a discarded sample must not write lastSample")
	require.False(t, ps.parked)
}

// racingProbe simulates a concurrent transition into RUNNING happening while
// the probe call is in flight, exercising onSessionParkedHook's
// same-critical-section revalidation.
type racingProbe struct {
	svc               *Service
	taskID, sessionID string
	result            string
}

func (p *racingProbe) ProbeBackgroundWorkloads(_ context.Context, _ string) (string, error) {
	p.svc.handleParkedStateTransition(
		context.Background(), p.taskID, p.sessionID,
		models.TaskSessionStateWaitingForInput, models.TaskSessionStateRunning,
	)
	return p.result, nil
}

// TestParked_V1_04_ExactlyOneProbePerRacingSettle verifies V1-04: when two
// settle paths race the same session's transition into WAITING_FOR_INPUT,
// exactly one wins the state CAS (persistTaskSessionState's conditional
// update) and exactly one synchronous probe is issued — no extra lock is
// added for this; the CAS itself is the serialization point (§7.2). Two real
// goroutines call updateTaskSessionState concurrently, joined via
// sync.WaitGroup rather than a sleep.
func TestParked_V1_04_ExactlyOneProbePerRacingSettle(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)
	attestShellLaunch(ctx, svc, "t1", "s1")

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)
		}()
	}
	wg.Wait()

	require.Equal(t, 1, probe.callCount(), "exactly one settle path should win the CAS and issue a probe")
	require.True(t, svc.ParkedProjectionSnapshot("s1"))
}

// firstPublishBlocksEventBus is a test double for bus.EventBus whose FIRST
// Publish call for a matching subject blocks until release is closed; every
// other call (including the earlier publishTaskSessionStateChanged call the
// same transition issues for events.TaskSessionStateChanged) passes through
// immediately. Used to force a genuine goroutine-scheduling window between a
// transition's session-level parkedStatesMu write (which happens before
// publishParkedChanged's events.TaskSessionActivityChanged publish) and its
// own updateTaskParkedState call (which happens after) — the exact window
// MUST-FIX 1 (review round 1) closed.
type firstPublishBlocksEventBus struct {
	subject string
	mu      sync.Mutex
	blocked bool
	entered chan struct{}
	release chan struct{}
}

func newFirstPublishBlocksEventBus(subject string) *firstPublishBlocksEventBus {
	return &firstPublishBlocksEventBus{
		subject: subject,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (b *firstPublishBlocksEventBus) Publish(_ context.Context, subject string, _ *bus.Event) error {
	if subject != b.subject {
		return nil
	}
	b.mu.Lock()
	first := !b.blocked
	b.blocked = true
	b.mu.Unlock()
	if first {
		close(b.entered)
		<-b.release
	}
	return nil
}

func (b *firstPublishBlocksEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *firstPublishBlocksEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *firstPublishBlocksEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (b *firstPublishBlocksEventBus) Close()            {}
func (b *firstPublishBlocksEventBus) IsConnected() bool { return true }

// TestParked_ConcurrentSettleAndResumeConvergeOnCurrentState is an
// adversarial Testing-phase extension of MUST-FIX 1 (review round 1): it
// reproduces the exact bug through real goroutine scheduling and the public
// API, rather than the direct internal-state manipulation Build's regression
// test used. A session settles to WAITING_FOR_INPUT and its own delayed
// task-level publish (blocked mid-flight via firstPublishBlocksEventBus) is
// still in flight when a second, real transition resumes the same session
// into RUNNING and completes its own task-level publish first. Without the
// fix (updateTaskParkedState re-reading the authoritative value at call
// time), the delayed settle's publish would resurrect a stale `true` after
// the resume already correctly published `false`.
func TestParked_ConcurrentSettleAndResumeConvergeOnCurrentState(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)
	attestShellLaunch(ctx, svc, "t1", "s1")

	eb := newFirstPublishBlocksEventBus(events.TaskSessionActivityChanged)
	svc.eventBus = eb

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)
	}()

	<-eb.entered // the settle's own task-level publish is now blocked, ps.parked already written true

	require.True(t, svc.ParkedProjectionSnapshot("s1"), "session-level write must have already landed while the settle's publish is blocked")

	// A second, real transition resumes the session out of WAITING_FOR_INPUT
	// while the settle's publish is still blocked. Its own Publish call is
	// the SECOND call to eb, so it passes through immediately — this
	// transition's updateTaskParkedState call completes in full before the
	// settle's delayed one resumes.
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateRunning, "", true)
	require.False(t, svc.ParkedProjectionSnapshot("s1"))
	require.False(t, svc.TaskParkedProjectionSnapshot("t1"), "resume's publish must land correctly before the settle's delayed publish resumes")

	close(eb.release) // let the settle's delayed publish (and its updateTaskParkedState call) proceed
	wg.Wait()

	require.False(t, svc.ParkedProjectionSnapshot("s1"), "session must not be parked after resuming")
	require.False(t, svc.TaskParkedProjectionSnapshot("t1"),
		"the settle's delayed task-level publish must not resurrect a stale true after the resume already published false")
}

// TestParked_TaskLevelOR verifies AC-49: task-level parked is the OR of its
// sessions' parked states, and un-parking one member while another stays
// parked keeps the task-level value true.
func TestParked_TaskLevelOR(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	// A second session on the SAME task: seedSession recreates the
	// workspace/workflow/task, so a second call for the same task would
	// collide on the workspace's unique ID — add the session row directly.
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "s2",
		TaskID:    "t1",
		State:     models.TaskSessionStateRunning,
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}))
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	svc.config.ParkedOnBackgroundWork = true
	svc.SetBackgroundProbe(probe)

	// Neither parked initially.
	require.False(t, svc.TaskParkedProjectionSnapshot("t1"))

	attestShellLaunch(ctx, svc, "t1", "s1")
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)
	require.True(t, svc.TaskParkedProjectionSnapshot("t1"), "S1 parking should flip the task-level OR true")

	attestShellLaunch(ctx, svc, "t1", "s2")
	svc.updateTaskSessionState(ctx, "t1", "s2", models.TaskSessionStateWaitingForInput, "", false)
	require.True(t, svc.TaskParkedProjectionSnapshot("t1"))

	// S2 un-parks (enters RUNNING); S1 is still parked, so the task stays true.
	svc.updateTaskSessionState(ctx, "t1", "s2", models.TaskSessionStateRunning, "", true)
	require.False(t, svc.ParkedProjectionSnapshot("s2"))
	require.True(t, svc.ParkedProjectionSnapshot("s1"))
	require.True(t, svc.TaskParkedProjectionSnapshot("t1"), "task must stay true while S1 is still parked")
}

// TestUpdateTaskParkedState_LateCallMustNotResurrectStaleValue closes
// MUST-FIX 1 from review round 1: onSessionParkedHook and
// handleParkedStateTransition each release parkedStatesMu, THEN call
// updateTaskParkedState. Two racing transitions for the SAME session issue
// two such calls with no ordering guarantee relative to each other — if the
// physically later call carried a caller-captured value from before the
// release, it could permanently clobber a correct, already-superseded one,
// since taskParkedState.members is never re-synced. updateTaskParkedState
// now re-reads ParkedProjectionSnapshot itself at call time instead of
// trusting an argument, so this reproduces the exact interleaving by hand: a
// later transition's publish (correctly false) runs first, then an earlier
// transition's delayed publish runs second — even though the session-level
// value was true when that earlier transition first observed it, the
// delayed call must read the CURRENT (false) value, not resurrect true.
func TestUpdateTaskParkedState_LateCallMustNotResurrectStaleValue(t *testing.T) {
	ctx := context.Background()
	svc := parkedTestService(t, "t1", "s1", &fakeBackgroundProbe{result: probeResultLive})
	attestShellLaunch(ctx, svc, "t1", "s1")

	svc.parkedStatesMu.Lock()
	svc.parkedStates["s1"].parked = true
	svc.parkedStatesMu.Unlock()

	// A later, logically-subsequent transition already flipped the
	// session-level value to false and published it first.
	svc.parkedStatesMu.Lock()
	svc.parkedStates["s1"].parked = false
	svc.parkedStatesMu.Unlock()
	svc.updateTaskParkedState(ctx, "t1", "s1")
	require.False(t, svc.TaskParkedProjectionSnapshot("t1"))

	// The earlier transition's own delayed publish call now runs. It reads
	// live state at call time, so it must see the CURRENT false, not the
	// true that was live when that earlier transition first observed it.
	svc.updateTaskParkedState(ctx, "t1", "s1")
	require.False(t, svc.TaskParkedProjectionSnapshot("t1"),
		"a delayed task-level publish must not resurrect a parked value a later transition already superseded")
}

// TestParked_TaskNoSessions verifies AC-50: a task with no recorded
// projection reports false.
func TestParked_TaskNoSessions(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	require.False(t, svc.TaskParkedProjectionSnapshot("no-such-task"))
}

// TestParked_QueuedPromptDoesNotUnpark verifies AC-75: a prompt that is
// queued but not admitted (the session stays WAITING_FOR_INPUT) does not
// change the parked projection; only actual admission into RUNNING clears it.
func TestParked_QueuedPromptDoesNotUnpark(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)
	attestShellLaunch(ctx, svc, "t1", "s1")
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)
	require.True(t, svc.ParkedProjectionSnapshot("s1"))

	// Queuing does not call updateTaskSessionState at all — the session
	// state is untouched, so the projection is untouched too. Directly
	// confirm the invariant: re-reading without any state transition leaves
	// parked true.
	require.True(t, svc.ParkedProjectionSnapshot("s1"))

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateRunning, "", true)
	require.False(t, svc.ParkedProjectionSnapshot("s1"), "admission into RUNNING clears it")
}

// TestParked_AC74_StaleAfterBackgroundWorkExitsUntilResume closes AC-74(V1)'s
// test debt (Review round 2 should-fix item 3): once a session is parked on
// its one synchronous settle sample, the background workload subsequently
// exiting must NOT clear the projection and must NOT trigger a further
// probe — V1 has no sampler, so parked stays true until the session actually
// leaves WAITING_FOR_INPUT. This is distinct from
// TestParked_QueuedPromptDoesNotUnpark (AC-75), which never varies the probe
// result and so cannot distinguish "correctly stale" from "accidentally
// never re-checked". Here the probe's own result is flipped to "settled"
// after parking to prove staleness is a deliberate feature, not an
// unexercised accident.
func TestParked_AC74_StaleAfterBackgroundWorkExitsUntilResume(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)
	attestShellLaunch(ctx, svc, "t1", "s1")

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)
	require.True(t, svc.ParkedProjectionSnapshot("s1"), "session should be parked")
	require.Equal(t, 1, probe.callCount())

	// The background workload exits — simulated by flipping the probe's own
	// result. AC-74(V1): no further probe is taken, so this change is never
	// observed and parked remains true indefinitely.
	probe.mu.Lock()
	probe.result = probeResultSettled
	probe.mu.Unlock()

	require.True(t, svc.ParkedProjectionSnapshot("s1"),
		"parked must remain true after the background workload exits — no sampler re-checks it (AC-74 V1)")
	require.Equal(t, 1, probe.callCount(), "no further probe may be issued while the session stays WAITING_FOR_INPUT")

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateRunning, "", true)
	require.False(t, svc.ParkedProjectionSnapshot("s1"), "leaving WAITING_FOR_INPUT clears it")
}

// panickingBackgroundProbe is a test double for backgroundProbePort whose
// ProbeBackgroundWorkloads always panics, for AC-46 condition 7.
type panickingBackgroundProbe struct{}

func (panickingBackgroundProbe) ProbeBackgroundWorkloads(context.Context, string) (string, error) {
	panic("probe port implementation panicked")
}

// TestRunProbe_ErrorAlongsideLiveValueResolvesToUnknown verifies AC-46
// condition 6: a probe port that returns a non-nil error alongside a "live"
// value must not have that value read — runProbe must resolve to unknown
// regardless, per the rule stated in runProbe's own doc comment ("the caller
// MUST NOT read the port's returned value when it also returned a non-nil
// error"). A port that violates its own contract this way is exactly the
// case this defensive check exists for.
func TestRunProbe_ErrorAlongsideLiveValueResolvesToUnknown(t *testing.T) {
	probe := &fakeBackgroundProbe{result: probeResultLive, err: errors.New("port contract violation")}
	svc := parkedTestService(t, "t1", "s1", probe)

	got := svc.runProbe(context.Background(), "s1")

	require.Equal(t, probeResultUnknown, got, "a non-nil error must force unknown even when the value is live")
}

// TestRunProbe_PanickingProbeResolvesToUnknown verifies AC-46 condition 7: a
// probe port implementation that panics must not crash the settle hook —
// runProbe's defer/recover must catch it and resolve to unknown.
func TestRunProbe_PanickingProbeResolvesToUnknown(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	svc.config.ParkedOnBackgroundWork = true
	svc.SetBackgroundProbe(panickingBackgroundProbe{})

	require.NotPanics(t, func() {
		got := svc.runProbe(context.Background(), "s1")
		require.Equal(t, probeResultUnknown, got, "a panicking probe must resolve to unknown, not crash the caller")
	})
}

// blockingTransportProbe is a test double for backgroundProbePort that
// simulates a stalled/half-open agentctl WebSocket connection: it never
// returns on its own, only when its own ctx is cancelled — exactly like the
// real sendStreamRequest's `select { case <-respCh: ...; case <-ctx.Done():
// ... }` behaves when the response frame never arrives (Review round 2
// MUST-FIX 1). Returns ctx.Err() so a caller that (incorrectly) read the
// error would still not fabricate a live/settled result.
type blockingTransportProbe struct{}

func (blockingTransportProbe) ProbeBackgroundWorkloads(ctx context.Context, _ string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// TestRunProbe_BlockedTransportResolvesToUnknownWithinBudget is the Review
// round 2 MUST-FIX 1 regression: runProbe's own doc comment used to claim
// "the whole round trip is already bounded without a second, speculative
// budget concept" — false, because the only context.WithTimeout in the
// chain lived inside agentctl's process.Manager.ProbeProcessTree, on the far
// side of the WebSocket boundary, bounding just the OS-level walk. Nothing
// bounded the orchestrator's own wait for the response to travel back over
// the wire, so a wedged/half-open agentctl connection blocked the settle
// transition indefinitely — violating spec §7.3 ("The settle transition is
// never delayed beyond the budget") and AC-40. runProbe must now apply its
// own context.WithTimeout(ctx, budget) around the port call so a transport
// that never responds still resolves to unknown within the configured
// budget, not the caller's (possibly deadline-free) ctx.
func TestRunProbe_BlockedTransportResolvesToUnknownWithinBudget(t *testing.T) {
	t.Setenv("KANDEV_PARKED_PROBE_BUDGET", "50ms")
	svc := parkedTestService(t, "t1", "s1", blockingTransportProbe{})

	resultCh := make(chan string, 1)
	start := time.Now()
	go func() {
		resultCh <- svc.runProbe(context.Background(), "s1")
	}()

	select {
	case got := <-resultCh:
		elapsed := time.Since(start)
		require.Equal(t, probeResultUnknown, got, "a wedged transport must resolve to unknown")
		require.Less(t, elapsed, 2*time.Second,
			"the settle transition must not be delayed beyond the budget (spec §7.3/AC-40), even when the caller's own ctx has no deadline")
	case <-time.After(2 * time.Second):
		t.Fatal("runProbe did not return within 2s of a wedged transport — the settle path has no independent timeout")
	}
}

// lastParkedOnBackgroundWorkValue returns the parked_on_background_work field
// from the most recently published events.TaskSessionActivityChanged event,
// failing the test if none was published.
func lastParkedOnBackgroundWorkValue(t *testing.T, eb *recordingEventBus) bool {
	t.Helper()
	for i := len(eb.events) - 1; i >= 0; i-- {
		rec := eb.events[i]
		if rec.subject != events.TaskSessionActivityChanged {
			continue
		}
		data, ok := rec.event.Data.(map[string]interface{})
		require.True(t, ok, "expected event.Data to be a map, got %T", rec.event.Data)
		v, ok := data["parked_on_background_work"]
		require.True(t, ok, "parked_on_background_work missing from session.activity_changed payload: %#v", data)
		b, ok := v.(bool)
		require.True(t, ok, "parked_on_background_work = %#v, want bool", v)
		return b
	}
	t.Fatal("no events.TaskSessionActivityChanged event was published")
	return false
}

// TestPublishForegroundActivityNow_CarriesParkedOnBackgroundWork closes the
// wire-level gap the Testing phase found: the session-level
// session.activity_changed carrier (turn_activity.go's
// publishForegroundActivityNow) must actually put parked_on_background_work
// on the published event, not just in the in-memory projection — mirroring
// the proof TestPublishTaskActivityIfChanged_EmitsOnParkedOnlyChange already
// gives the task-level carrier.
func TestPublishForegroundActivityNow_CarriesParkedOnBackgroundWork(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)
	eb := &recordingEventBus{}
	svc.eventBus = eb

	attestShellLaunch(ctx, svc, "t1", "s1")
	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)
	require.True(t, svc.ParkedProjectionSnapshot("s1"), "precondition: session should be parked")

	svc.publishForegroundActivityNow(ctx, "t1", "s1", nil, 0)
	require.True(t, lastParkedOnBackgroundWorkValue(t, eb), "parked session must publish parked_on_background_work=true on the wire")

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateRunning, "", true)
	require.False(t, svc.ParkedProjectionSnapshot("s1"), "precondition: session should have un-parked")

	svc.publishForegroundActivityNow(ctx, "t1", "s1", nil, 0)
	require.False(t, lastParkedOnBackgroundWorkValue(t, eb), "un-parked session must publish parked_on_background_work=false on the wire")
}
