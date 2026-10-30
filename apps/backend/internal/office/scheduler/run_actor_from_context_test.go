package scheduler

// Internal (white-box) test file: actorFromRunContext is unexported.

import (
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
	runsmodels "github.com/kandev/kandev/internal/runs/models"

	"github.com/kandev/kandev/internal/office/shared"
)

// TestActorFromRunContext_MirrorsNormalizeActorsDowngradeRule pins Review
// round 16's RR16-F3: actorFromRunContext downgraded ANY actor with an
// empty ActorID to ActorKindSystem, before even checking ActorType — a
// stricter rule than runs/service.normalizeActor, which this function's
// own doc comment claims to mirror. normalizeActor only downgrades an
// invalid/absent ActorType or an ActorKindAgent actor with no ActorID; a
// "user" actor is never downgraded for a missing ActorID, because a
// browser status/assignee change has no ActorID to give (agentIDFromCtx
// reads the agent_caller JWT claim, which browsers never carry) — the old
// stricter rule silently reclassified every such wake as system, losing
// human priority and the AC-005.6 budget exemption
// (AC-OFFICE-RUN-CAUSATION-001.16, which also requires the downgrade to be
// counted so a caller failing to declare its actor stays visible).
func TestActorFromRunContext_MirrorsNormalizeActorsDowngradeRule(t *testing.T) {
	cases := []struct {
		name        string
		actorType   string
		actorID     string
		wantKind    runsmodels.ActorKind
		wantID      string
		wantCounted bool
	}{
		{"user_with_id", "user", "user-1", runsmodels.ActorKindUser, "user-1", false},
		{"user_without_id_is_not_downgraded", "user", "", runsmodels.ActorKindUser, "", false},
		{"agent_with_id", "agent", "agent-1", runsmodels.ActorKindAgent, "agent-1", false},
		{"agent_without_id_is_downgraded", "agent", "", runsmodels.ActorKindSystem, "", true},
		{"empty_type_is_downgraded", "", "", runsmodels.ActorKindSystem, "", true},
		{"unknown_type_with_id_is_still_downgraded", "bogus", "id-1", runsmodels.ActorKindSystem, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := "actor-ctx-test-" + tc.name
			prefix := "reason=" + reason
			before := readCounter(t, shared.LaunchActorMissingTotal, prefix)

			kind, id := actorFromRunContext(RunContext{
				Reason:    reason,
				ActorType: tc.actorType,
				ActorID:   tc.actorID,
			})

			if kind != tc.wantKind || id != tc.wantID {
				t.Fatalf("actorFromRunContext(type=%q, id=%q) = (%v, %q), want (%v, %q)",
					tc.actorType, tc.actorID, kind, id, tc.wantKind, tc.wantID)
			}
			after := readCounter(t, shared.LaunchActorMissingTotal, prefix)
			if gotCounted := after > before; gotCounted != tc.wantCounted {
				t.Errorf("counted = %v, want %v (office_launch_actor_missing_total delta %d->%d)",
					gotCounted, tc.wantCounted, before, after)
			}
		})
	}
}

// Task-owned attempts keep the existing provider cap even when the run carries
// the Office task-capacity context; only taskless Office capacity waits are
// exempt from spending the budget.
func TestProviderAttemptCountKeepsTaskOwnedCap(t *testing.T) {
	prior := []officemodels.RouteAttempt{
		{Outcome: RouteAttemptOutcomeDeferredCapacity},
		{Outcome: RouteAttemptOutcomeLaunched},
	}
	run := &runsmodels.Run{Payload: `{"task_id":"t-1"}`}
	if got := providerAttemptCount(run, prior); got != len(prior) {
		t.Fatalf("task-owned providerAttemptCount = %d, want %d", got, len(prior))
	}
}

// A taskless Office capacity wait reuses its existing attempt instead of
// consuming provider-attempt budget.
func TestProviderAttemptCountSkipsOfficeCapacityWait(t *testing.T) {
	prior := []officemodels.RouteAttempt{
		{Outcome: RouteAttemptOutcomeDeferredCapacity},
		{Outcome: RouteAttemptOutcomeLaunched},
	}
	run := &runsmodels.Run{Payload: `{}`}
	if got := providerAttemptCount(run, prior); got != 1 {
		t.Fatalf("taskless providerAttemptCount = %d, want 1", got)
	}
}

// The queued launch origin and resolved workflow selection are preserved by
// recognising the unchanged deferred candidate as the same waiting identity.
func TestSameWaitingCandidatePreservesQueuedSelection(t *testing.T) {
	candidate := routing.Candidate{ExecutionProfileID: "p-1", ProviderID: routing.ProviderID("claude-acp"), Model: "m-1"}
	attempt := officemodels.RouteAttempt{
		Outcome: RouteAttemptOutcomeDeferredCapacity, ExecutionProfileID: "p-1",
		ProviderID: "claude-acp", Model: "m-1", Tier: "balanced", TierSource: "role",
	}
	if !sameWaitingCandidate(attempt, candidate, routing.Tier("balanced"), "role") {
		t.Fatal("same deferred candidate should match")
	}
	if sameWaitingCandidate(attempt, candidate, routing.Tier("frontier"), "role") {
		t.Fatal("a changed tier must not match the waiting candidate")
	}
}
