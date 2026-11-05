package sandboxruntime

import (
	"testing"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestExecutionEvidenceRequiresFreshPolicyProcessAndAttemptBinding(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	attempt := &policy.Attempt{ID: "attempt", State: policy.AttemptRunning,
		Snapshot: policy.AttemptSnapshot{Launch: &contract.Launch{Generation: 2, PolicyDigest: "policy"}},
		Owner:    &policy.AttemptOwner{SupervisorPID: 123, SupervisorBirth: "birth"}}
	valid := func() *pluginsdk.SandboxStatusEvidence {
		return &pluginsdk.SandboxStatusEvidence{
			SchemaVersion: 1, Mechanism: "fixture-kernel", AttemptId: "attempt", Generation: 2,
			PolicyDigest: "policy", Process: &pluginsdk.SandboxProcessIdentity{Pid: 123, BirthId: "birth"},
			ObservedAtUnixMs: now.Add(-time.Second).UnixMilli(),
		}
	}
	require.True(t, boundConfinementEvidence(valid(), attempt, now), "fresh exact evidence is eligible only after core verifies live ownership")
	for _, test := range []struct {
		name   string
		change func(*pluginsdk.SandboxStatusEvidence)
	}{
		{"schema", func(e *pluginsdk.SandboxStatusEvidence) { e.SchemaVersion = 2 }},
		{"attempt", func(e *pluginsdk.SandboxStatusEvidence) { e.AttemptId = "sibling" }},
		{"generation", func(e *pluginsdk.SandboxStatusEvidence) { e.Generation++ }},
		{"policy", func(e *pluginsdk.SandboxStatusEvidence) { e.PolicyDigest = "other" }},
		{"process", func(e *pluginsdk.SandboxStatusEvidence) { e.Process.Pid++ }},
		{"birth", func(e *pluginsdk.SandboxStatusEvidence) { e.Process.BirthId = "reused-pid" }},
		{"missing-process", func(e *pluginsdk.SandboxStatusEvidence) { e.Process = nil }},
		{"expired", func(e *pluginsdk.SandboxStatusEvidence) { e.ObservedAtUnixMs = now.Add(-30 * time.Second).UnixMilli() }},
		{"future", func(e *pluginsdk.SandboxStatusEvidence) { e.ObservedAtUnixMs = now.Add(time.Millisecond).UnixMilli() }},
		{"no-time", func(e *pluginsdk.SandboxStatusEvidence) { e.ObservedAtUnixMs = 0 }},
		{"no-mechanism", func(e *pluginsdk.SandboxStatusEvidence) { e.Mechanism = "" }},
		{"private-mechanism", func(e *pluginsdk.SandboxStatusEvidence) { e.Mechanism = "/private/state?token=value" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := valid()
			test.change(e)
			require.False(t, boundConfinementEvidence(e, attempt, now))
		})
	}
	for _, state := range []string{policy.AttemptPrepared, policy.AttemptStarting, policy.AttemptStopped, policy.AttemptFailed, policy.AttemptRecoveryRequired} {
		attempt.State = state
		require.False(t, boundConfinementEvidence(valid(), attempt, now), state)
	}
	require.False(t, boundConfinementEvidence(nil, attempt, now))
	require.False(t, boundConfinementEvidence(valid(), nil, now))
}
