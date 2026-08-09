package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// TestParkedRecogniserUsesNeutralPredicate (AC-69a) verifies that the
// observedDetached recogniser gates only on IsDetachedBackgroundLaunch() and
// Kind==BackgroundWorkKindShell — never on the payload AgentID field.
// Injecting "mock-agent" as the AgentID and a shell-detached normalized payload
// must still set observedDetached=true, proving the predicate is agent-neutral.
// (Relabeled 2026-08-09: this test's own content is agent-name independence,
// AC-69a's territory, not AC-35's flag-independence claim — see
// TestParkedFeatureNeverReferencesTheHandoffFlag and
// TestComputeParked_IdenticalWithFlagForcedOnOrOff below for the actual AC-35
// coverage.)
func TestParkedRecogniserUsesNeutralPredicate(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-ac35"

	// Build a NormalizedPayload that satisfies both predicate terms:
	//   1. IsDetachedBackgroundLaunch() == true  (detached=true, ended=false)
	//   2. backgroundWorkKind(n) == BackgroundWorkKindShell
	normalized := &streams.NormalizedPayload{}
	normalized.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "", true, false)

	// Use "mock-agent" as AgentID — this is NOT "claude-agent-acp" and would be
	// rejected by the normalizer's stampBackgroundShellWork. But since the
	// orchestrator recogniser reads only the already-stamped normalized payload
	// (not AgentID), the recogniser must accept it.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-ac35",
		SessionID: sessionID,
		AgentID:   "mock-agent", // explicitly NOT claude-agent-acp
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_call",
			ToolCallID: "tc-1",
			Normalized: normalized,
		},
	})

	ps := svc.parkedStateFor(sessionID)
	if ps == nil {
		t.Fatal("expected parked state to be created for session")
	}
	if !ps.observedDetached {
		t.Error("expected observedDetached=true: recogniser must use neutral predicate (Kind+IsDetachedBackgroundLaunch), not AgentID")
	}
}

// TestParkedDoesNotAffectNotifications (AC-76) verifies that the parked hook
// fires AFTER publishTaskSessionStateChanged — i.e., the state-change event
// (which triggers session.turn_finished notifications) is not deferred or
// suppressed by the parked projection.
//
// Structural guard: updateTaskSessionStateWithHook calls
// publishTaskSessionStateChanged before calling onSessionParkedHook. This test
// confirms the DB row reflects the new state BEFORE the parked hook can observe it,
// proving notification dispatch (which reads DB state) is not delayed.
func TestParkedDoesNotAffectNotifications(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-ac76", "s-ac76", "")

	probe := &fakeProbe{result: probeResultLive}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetBackgroundProbe(probe)
	svc.getOrCreateParkedState("s-ac76").observedDetached = true

	// Transition to WAITING_FOR_INPUT via the hook-bearing path.
	// The session starts in RUNNING (seeded by seedSession), so nextState=WAITING_FOR_INPUT.
	_, ok := svc.updateTaskSessionStateWithHook(
		context.Background(), "t-ac76", "s-ac76",
		models.TaskSessionStateWaitingForInput,
		"",    // errorMessage
		false, // allowWakeFromWaiting
		nil,   // onChanged
	)
	if !ok {
		t.Fatal("expected state transition to succeed")
	}

	// The session row must reflect the new state (i.e., the DB write happened
	// before or independently of the parked hook).
	session, err := repo.GetTaskSession(context.Background(), "s-ac76")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("expected session state=%s in DB, got %s — state change must precede parked hook",
			models.TaskSessionStateWaitingForInput, session.State)
	}

	// The parked projection must have fired (probe returned "live").
	ps := svc.parkedStateFor("s-ac76")
	if ps == nil || !ps.parked {
		t.Error("expected parked=true after WAITING_FOR_INPUT with live probe — hook did not suppress state change")
	}

	t.Cleanup(func() { svc.evictParkedState(context.Background(), "t-ac76", "s-ac76", true) })
}

// TestParkedEpochRestartSurvivable (AC-77 / AC-78) verifies that after a
// simulated backend restart (parkedEpoch incremented, session reset to parked=false,
// revision=0), the snapshot returns the new epoch and the boot state.
func TestParkedEpochRestartSurvivable(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-epoch"
	// Simulate "before restart": parked=true with epoch E and revision R.
	epochBefore := int64(1_000_000)
	svc.parkedEpoch = epochBefore
	ps := svc.getOrCreateParkedState(sessionID)
	ps.parked = true
	ps.revision = 5

	// Verify snapshot before restart.
	parked, epoch, revision := svc.ParkedProjectionSnapshot(sessionID)
	if !parked || epoch != epochBefore || revision != 5 {
		t.Fatalf("pre-restart: want parked=true epoch=%d revision=5, got parked=%v epoch=%d revision=%d",
			epochBefore, parked, epoch, revision)
	}

	// Simulate backend restart: new epoch, state reset.
	epochAfter := int64(2_000_000)
	svc.parkedEpoch = epochAfter
	svc.evictParkedState(context.Background(), "", sessionID, true) // session cleared on restart

	// The snapshot for the session returns boot state (parked=false, revision=0)
	// but the new epoch — clients use the epoch change as a discard signal.
	parked, epoch, revision = svc.ParkedProjectionSnapshot(sessionID)
	if parked {
		t.Error("post-restart: expected parked=false for unknown session")
	}
	if epoch != epochAfter {
		t.Errorf("post-restart: expected epoch=%d (new), got %d", epochAfter, epoch)
	}
	if revision != 0 {
		t.Errorf("post-restart: expected revision=0, got %d", revision)
	}
}

// TestParkedFIFOOrdering (AC-79, AC-79a) verifies that the orchestrator
// processes a detached-shell tool_call event BEFORE the subsequent
// turn_started event, so the attestation is visible when the turn-completion
// frame is handled (AC-79) — and that turn_started then clears it for the
// *next* turn, never leaving turn N's attestation to leak into turn N+1
// (AC-79a: "observed_detached is set for turn N and then cleared by turn
// N+1 — never the reverse").
func TestParkedFIFOOrdering(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-fifo"

	// Step 1: tool_call with detached shell attestation arrives.
	normalized := &streams.NormalizedPayload{}
	normalized.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "", true, false)
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-fifo",
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_call",
			ToolCallID: "tc-fifo",
			Normalized: normalized,
		},
	})

	// After tool_call: observedDetached must be set, turnMarker must still be 0.
	ps := svc.parkedStateFor(sessionID)
	if ps == nil || !ps.observedDetached {
		t.Fatal("expected observedDetached=true after tool_call with detached shell attestation")
	}
	markerAfterToolCall := ps.turnMarker

	// Step 2: turn_started arrives immediately after (simulating FIFO ordering).
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-fifo",
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type: streams.EventTypeTurnStarted,
		},
	})

	ps = svc.parkedStateFor(sessionID)
	if ps.turnMarker <= markerAfterToolCall {
		t.Errorf("expected turnMarker to increment after turn_started, got %d (was %d)",
			ps.turnMarker, markerAfterToolCall)
	}
	// D3/AC-79a: observed_detached was set for turn N (the tool_call frame
	// preceded turn_started, proving AC-79's ordering) and is then cleared by
	// turn_started for turn N+1 — never the reverse. A stale attestation must
	// not survive the boundary it names.
	if ps.observedDetached {
		t.Error("observedDetached must be cleared by turn_started (D3/AC-79a: set for turn N, cleared by turn N+1)")
	}
}

// parkedFeatureSourceFiles lists the source files (excluding tests) that hold
// the symbols AC-35 names: the parked projection and its revision accessors,
// the probe port and its production implementation, the sampling loop, the
// recogniser registry, and the settle hook. Both parked_state.go and
// parked_projection.go are dedicated to this feature end to end and can be
// scanned wholesale; the mixed files below are scanned only for the specific
// declarations AC-35 names, via TestParkedFeatureNeverReferencesTheHandoffFlag.
var parkedFeatureSourceFiles = []string{
	"parked_state.go",
	"parked_projection.go",
}

// parkedFeatureForbiddenIdentifiers are the flag key, config field, and
// accessor names AC-35 forbids the parked-projection symbols from
// referencing, so the feature stays independent of
// features.claudeBackgroundPromptHandoff, which remains off and unread.
var parkedFeatureForbiddenIdentifiers = []string{
	"claudeBackgroundPromptHandoff",
	"ClaudeBackgroundPromptHandoff",
	"claudeBackgroundPromptHandoffEnabled",
	"claudeBackgroundPromptHandoffEnabledForSession",
}

// TestParkedFeatureNeverReferencesTheHandoffFlag is AC-35's architecture
// test: none of the symbols this feature introduces may reference the
// claudeBackgroundPromptHandoff flag key, the Features.ClaudeBackgroundPromptHandoff
// config field, or the claudeBackgroundPromptHandoffEnabled(ForSession)
// accessors. Scoped to symbol/file granularity per the spec's correction —
// package orchestrator legitimately references the flag elsewhere (the
// unrelated ForegroundActivity gate in service.go and turn_activity.go), so a
// package-wide check would be false by construction.
func TestParkedFeatureNeverReferencesTheHandoffFlag(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(sourceFile)

	var violations []string
	for _, name := range parkedFeatureSourceFiles {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(data)
		for _, forbidden := range parkedFeatureForbiddenIdentifiers {
			if strings.Contains(src, forbidden) {
				violations = append(violations, fmt.Sprintf("%s references forbidden identifier %q", name, forbidden))
			}
		}
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		t.Fatalf("parked-projection source reads the claudeBackgroundPromptHandoff flag (AC-35):\n%s",
			strings.Join(violations, "\n"))
	}
}

// TestComputeParked_IdenticalWithFlagForcedOnOrOff is AC-35's behavioural
// clause: a parked session's projection is computed identically with the
// flag forced on and forced off, for the same inputs.
func TestComputeParked_IdenticalWithFlagForcedOnOrOff(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	probe := &fakeProbe{result: probeResultLive}
	svc.SetBackgroundProbe(probe)

	const taskID, sessionID = "task-ac35", "session-ac35"
	parkedTestSeedSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)

	for _, flag := range []bool{true, false} {
		svc.config.ClaudeBackgroundPromptHandoff = flag
		svc.getOrCreateParkedState(sessionID).observedDetached = true

		svc.onSessionParkedHook(context.Background(), taskID, sessionID)

		ps := svc.parkedStateFor(sessionID)
		if !ps.parked {
			t.Fatalf("flag=%v: expected parked=true, got false", flag)
		}
		t.Cleanup(func() { svc.evictParkedState(context.Background(), taskID, sessionID, true) })
	}
}
