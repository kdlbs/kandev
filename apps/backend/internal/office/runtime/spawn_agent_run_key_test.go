package runtime

import (
	"context"
	"errors"
	"expvar"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// spawnAgentRunKeylessCounterHasLabel reports whether the process-global
// office_run_dedup_keyless_total expvar map carries an entry for the given
// reason and cause. Mirrors internal/runs/service's own counterHasLabel test
// helper — that one lives in a different package (service_test) and cannot
// be imported here.
func spawnAgentRunKeylessCounterHasLabel(t *testing.T, reason, cause string) bool {
	t.Helper()
	v := expvar.Get("office_run_dedup_keyless_total")
	if v == nil {
		t.Fatalf("expvar map office_run_dedup_keyless_total not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("office_run_dedup_keyless_total is not a *expvar.Map")
	}
	found := false
	m.Do(func(kv expvar.KeyValue) {
		if strings.Contains(kv.Key, "reason="+reason) && strings.Contains(kv.Key, "cause="+cause) {
			found = true
		}
	})
	return found
}

// TestActionsSpawnAgentRun_PrefixesKeyWithCallerRunID is the AC-001.7
// regression: a non-empty agent-supplied key is prefixed with the calling
// run's id (agent:<callerRunID>:<key>) before reaching QueueRun, so a retry
// of the same run reuses the run id and still dedupes, while a later run
// (different RunID) produces a different key and is not suppressed. Before
// this test, the only coverage touching SpawnAgentRun's key logic was the
// unrelated cross-workspace denial test — the prefix itself was unpinned.
func TestActionsSpawnAgentRun_PrefixesKeyWithCallerRunID(t *testing.T) {
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-1"},
		},
	}
	runs := &recordingRunSpawner{}
	actions := NewActions(ActionDependencies{Runs: runs, AgentModifier: agents})
	runCtx := RunContext{
		WorkspaceID: "ws-1",
		RunID:       "run-caller-1",
		Capabilities: Capabilities{
			CanSpawnAgentRun: true,
		},
	}

	if err := actions.SpawnAgentRun(context.Background(), runCtx, SpawnAgentRunInput{
		AgentID:        "agent-2",
		Reason:         "custom_reason",
		IdempotencyKey: "custom-key",
	}); err != nil {
		t.Fatalf("SpawnAgentRun: %v", err)
	}

	if len(runs.calls) != 1 {
		t.Fatalf("run spawner calls = %d, want 1", len(runs.calls))
	}
	want := "agent:run-caller-1:custom-key"
	if got := runs.calls[0].IdempotencyKey; got != want {
		t.Fatalf("idempotency key = %q, want %q", got, want)
	}
}

// TestActionsSpawnAgentRun_DifferentCallerRun_ProducesDifferentKey proves
// the prefix's other half: the SAME agent-supplied key from a DIFFERENT
// calling run does not collide, because the caller run id varies the key.
func TestActionsSpawnAgentRun_DifferentCallerRun_ProducesDifferentKey(t *testing.T) {
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-1"},
		},
	}
	runs := &recordingRunSpawner{}
	actions := NewActions(ActionDependencies{Runs: runs, AgentModifier: agents})
	caps := Capabilities{CanSpawnAgentRun: true}

	for _, callerRunID := range []string{"run-a", "run-b"} {
		runCtx := RunContext{WorkspaceID: "ws-1", RunID: callerRunID, Capabilities: caps}
		if err := actions.SpawnAgentRun(context.Background(), runCtx, SpawnAgentRunInput{
			AgentID:        "agent-2",
			Reason:         "custom_reason",
			IdempotencyKey: "same-key",
		}); err != nil {
			t.Fatalf("SpawnAgentRun (caller %s): %v", callerRunID, err)
		}
	}

	if len(runs.calls) != 2 {
		t.Fatalf("run spawner calls = %d, want 2", len(runs.calls))
	}
	if runs.calls[0].IdempotencyKey == runs.calls[1].IdempotencyKey {
		t.Fatalf("two different caller runs produced the same key %q", runs.calls[0].IdempotencyKey)
	}
}

// TestActionsSpawnAgentRun_EmptyKey_NotPrefixed_ByDesign proves the third
// branch: the agent expressed no dedup intent (empty IdempotencyKey), so the
// call enqueues keyless without a prefix — prefixing an empty key would
// collapse every no-dedup-intent call inside one run onto a single key,
// suppressing every call after the first.
func TestActionsSpawnAgentRun_EmptyKey_NotPrefixed_ByDesign(t *testing.T) {
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-1"},
		},
	}
	runs := &recordingRunSpawner{}
	actions := NewActions(ActionDependencies{Runs: runs, AgentModifier: agents})
	runCtx := RunContext{
		WorkspaceID:  "ws-1",
		RunID:        "run-caller-2",
		Capabilities: Capabilities{CanSpawnAgentRun: true},
	}

	if err := actions.SpawnAgentRun(context.Background(), runCtx, SpawnAgentRunInput{
		AgentID: "agent-2",
		Reason:  "test_spawn_empty_key",
	}); err != nil {
		t.Fatalf("SpawnAgentRun: %v", err)
	}

	if len(runs.calls) != 1 {
		t.Fatalf("run spawner calls = %d, want 1", len(runs.calls))
	}
	if got := runs.calls[0].IdempotencyKey; got != "" {
		t.Fatalf("idempotency key = %q, want empty (keyless by design)", got)
	}
	if !spawnAgentRunKeylessCounterHasLabel(t, "test_spawn_empty_key", "by_design") {
		t.Fatal("expected office_run_dedup_keyless_total to carry a by_design entry for this reason")
	}
}

// TestActionsSpawnAgentRun_NoCallerRunID_EnqueuesKeylessUnresolved proves
// the fourth branch: a non-empty agent-supplied key with no caller run id to
// prefix it enqueues keyless (cause=unresolved) rather than risk colliding
// across unrelated runs.
func TestActionsSpawnAgentRun_NoCallerRunID_EnqueuesKeylessUnresolved(t *testing.T) {
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-1"},
		},
	}
	runs := &recordingRunSpawner{}
	actions := NewActions(ActionDependencies{Runs: runs, AgentModifier: agents})
	runCtx := RunContext{
		WorkspaceID:  "ws-1",
		RunID:        "",
		Capabilities: Capabilities{CanSpawnAgentRun: true},
	}

	if err := actions.SpawnAgentRun(context.Background(), runCtx, SpawnAgentRunInput{
		AgentID:        "agent-2",
		Reason:         "test_spawn_no_caller_run",
		IdempotencyKey: "custom-key",
	}); err != nil {
		t.Fatalf("SpawnAgentRun: %v", err)
	}

	if len(runs.calls) != 1 {
		t.Fatalf("run spawner calls = %d, want 1", len(runs.calls))
	}
	if got := runs.calls[0].IdempotencyKey; got != "" {
		t.Fatalf("idempotency key = %q, want empty (keyless, no caller run to prefix with)", got)
	}
	if !spawnAgentRunKeylessCounterHasLabel(t, "test_spawn_no_caller_run", "unresolved") {
		t.Fatal("expected office_run_dedup_keyless_total to carry an unresolved entry")
	}
}

// TestActionsSpawnAgentRun_ReasonTooLong_Rejected proves Reason is bounded
// before it can reach office_run_dedup_total /
// office_run_dedup_keyless_total as an expvar.Map label: those maps never
// evict entries, so an unvalidated agent-supplied Reason would let a caller
// grow them without limit. An over-length Reason is rejected with
// ErrReasonTooLong and never reaches the run spawner.
func TestActionsSpawnAgentRun_ReasonTooLong_Rejected(t *testing.T) {
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-1"},
		},
	}
	runs := &recordingRunSpawner{}
	actions := NewActions(ActionDependencies{Runs: runs, AgentModifier: agents})
	runCtx := RunContext{
		WorkspaceID:  "ws-1",
		RunID:        "run-caller-3",
		Capabilities: Capabilities{CanSpawnAgentRun: true},
	}

	err := actions.SpawnAgentRun(context.Background(), runCtx, SpawnAgentRunInput{
		AgentID: "agent-2",
		Reason:  strings.Repeat("a", maxSpawnAgentRunReasonLength+1),
	})
	if !errors.Is(err, ErrReasonTooLong) {
		t.Fatalf("SpawnAgentRun error = %v, want ErrReasonTooLong", err)
	}
	if len(runs.calls) != 0 {
		t.Fatalf("run spawner calls = %d, want 0 (rejected before enqueue)", len(runs.calls))
	}
}
