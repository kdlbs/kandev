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
// the flag forced on, and a fake probe.
func parkedTestService(t *testing.T, taskID, sessionID string, probe *fakeBackgroundProbe) *Service {
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

		svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

		require.True(t, svc.ParkedProjectionSnapshot("s1"), "session should be parked")
		require.True(t, svc.TaskParkedProjectionSnapshot("t1"), "task should be parked")
		require.Equal(t, 1, probe.callCount())
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
// kind is NOT shell (here: a subagent) never attests, so a later settle
// issues zero probes and never parks — even though IsDetachedBackgroundLaunch
// alone cannot tell the two kinds apart, the explicit Kind==shell filter can.
func TestParked_NoRecogniserMatch(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t1", "s1", probe)

	svc.handleToolCallEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			ToolCallID: "tc-subagent-1",
			ToolStatus: "running",
			Normalized: attestedSubagentPayload("desc", "prompt", "general"),
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
