package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// getRun reads back a single run row by agent + reason for assertions.
// Tests in this file each use a distinct (agent, reason) pair, so this
// stays unambiguous without threading the generated run id through
// QueueRun's return value (which only reports QueueOutcome).
func getRun(t *testing.T, repo *runssqlite.Repository, agentProfileID, reason string) *models.Run {
	t.Helper()
	var run models.Run
	err := repo.Reader().GetContext(context.Background(), &run,
		`SELECT * FROM runs WHERE agent_profile_id = ? AND reason = ?`,
		agentProfileID, reason)
	if err != nil {
		t.Fatalf("read back run: %v", err)
	}
	return &run
}

// TestQueueRun_RootRunStampsOwnIDAsCausationID pins
// AC-OFFICE-RUN-CAUSATION-001.2: a root cause (no CausingRunID) has its
// own id as its causation id, depth 0, and no parent.
func TestQueueRun_RootRunStampsOwnIDAsCausationID(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "root_reason",
		ActorKind:      models.ActorKindSystem,
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	run := getRun(t, repo, "agent-primary", "root_reason")
	if run.CausationID != run.ID {
		t.Fatalf("causation_id = %q, want own id %q", run.CausationID, run.ID)
	}
	if run.CausationDepth != 0 {
		t.Fatalf("causation_depth = %d, want 0", run.CausationDepth)
	}
	if run.ParentRunID != "" {
		t.Fatalf("parent_run_id = %q, want empty", run.ParentRunID)
	}
}

// TestQueueRun_InheritsCausationFromCausingRun pins
// AC-OFFICE-RUN-CAUSATION-001.3/.8: a nested enqueue inherits the
// causing run's causation id, human_rooted flag, and routine, with
// depth incremented by one.
func TestQueueRun_InheritsCausationFromCausingRun(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "parent_reason",
		ActorKind:      models.ActorKindUser,
		ActorID:        "user-1",
	}); err != nil {
		t.Fatalf("queue parent: %v", err)
	}
	parent := getRun(t, repo, "agent-primary", "parent_reason")
	if !parent.HumanRooted {
		t.Fatalf("parent human_rooted = false, want true (actor is user)")
	}

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "child_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "some-other-agent",
		CausingRunID:   parent.ID,
	}); err != nil {
		t.Fatalf("queue child: %v", err)
	}
	child := getRun(t, repo, "agent-primary", "child_reason")
	if child.CausationID != parent.CausationID {
		t.Fatalf("child causation_id = %q, want parent's %q", child.CausationID, parent.CausationID)
	}
	if child.ParentRunID != parent.ID {
		t.Fatalf("child parent_run_id = %q, want %q", child.ParentRunID, parent.ID)
	}
	if child.CausationDepth != parent.CausationDepth+1 {
		t.Fatalf("child causation_depth = %d, want %d", child.CausationDepth, parent.CausationDepth+1)
	}
	if !child.HumanRooted {
		t.Fatalf("child human_rooted = false, want inherited true")
	}
}

// TestQueueRun_HumanActorAlwaysRootsNewChain pins
// AC-OFFICE-RUN-CAUSATION-001.9: an actor who is human roots a new
// causation chain even when a CausingRunID is supplied.
func TestQueueRun_HumanActorAlwaysRootsNewChain(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "agent_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
	}); err != nil {
		t.Fatalf("queue causing run: %v", err)
	}
	causing := getRun(t, repo, "agent-primary", "agent_reason")

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "human_followup",
		ActorKind:      models.ActorKindUser,
		ActorID:        "user-1",
		CausingRunID:   causing.ID,
	}); err != nil {
		t.Fatalf("queue human follow-up: %v", err)
	}
	followUp := getRun(t, repo, "agent-primary", "human_followup")
	if followUp.CausationID == causing.CausationID {
		t.Fatalf("human-actor follow-up adopted causing run's chain, want a new root")
	}
	if followUp.CausationDepth != 0 {
		t.Fatalf("human-actor follow-up causation_depth = %d, want 0 (new root)", followUp.CausationDepth)
	}
	if !followUp.HumanRooted {
		t.Fatalf("human-actor follow-up human_rooted = false, want true")
	}
}

// TestQueueRun_RefusesUnknownAgentProfile pins
// AC-OFFICE-RUN-CAUSATION-001.20: an agent profile with no resolvable
// workspace refuses the enqueue rather than inserting a row with an
// empty workspace_id.
func TestQueueRun_RefusesUnknownAgentProfile(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "no-such-agent",
		Reason:         "task_assigned",
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue: err = %v, want *RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalWorkspaceMissing {
		t.Fatalf("refusal gate = %q, want %q", refusal.Gate, runsservice.RefusalWorkspaceMissing)
	}
}

// TestQueueRun_RefusesUnreadableCausingRun pins
// AC-OFFICE-RUN-CAUSATION-001.21: a CausingRunID that cannot be read
// refuses the enqueue rather than silently rooting it.
func TestQueueRun_RefusesUnreadableCausingRun(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "task_assigned",
		CausingRunID:   "no-such-run",
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue: err = %v, want *RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalCausingRunUnreadable {
		t.Fatalf("refusal gate = %q, want %q", refusal.Gate, runsservice.RefusalCausingRunUnreadable)
	}
}

// TestQueueRun_RefusesBeyondMaxCausationDepth pins
// AC-OFFICE-LAUNCH-SAFETY-003.4: an enqueue whose resolved depth
// exceeds the configured ceiling is refused, not inserted.
func TestQueueRun_RefusesBeyondMaxCausationDepth(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	svc.SetLaunchSafetyLimits(1, runsservice.DefaultSelfTriggerAllowance, runsservice.DefaultSelfTriggerTotalAllowance)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "depth0",
		ActorKind:      models.ActorKindSystem,
	}); err != nil {
		t.Fatalf("queue depth0: %v", err)
	}
	depth0 := getRun(t, repo, "agent-primary", "depth0")

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "depth1",
		ActorKind:      models.ActorKindSystem,
		CausingRunID:   depth0.ID,
	}); err != nil {
		t.Fatalf("queue depth1 (within limit): %v", err)
	}
	depth1 := getRun(t, repo, "agent-primary", "depth1")

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "depth2",
		ActorKind:      models.ActorKindSystem,
		CausingRunID:   depth1.ID,
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue depth2: err = %v, want *RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalCausationDepth {
		t.Fatalf("refusal gate = %q, want %q", refusal.Gate, runsservice.RefusalCausationDepth)
	}
}

// TestQueueRun_RefusesSelfTriggerBeyondAllowance pins
// AC-OFFICE-LAUNCH-SAFETY-004: an agent that has re-triggered itself
// with the same reason past the allowance within the rolling window is
// refused, and requests below the allowance succeed. Idempotency keys
// use the task-comment prefix so each request bypasses the 5s
// coalescing window (AC-OFFICE-LAUNCH-SAFETY-003.10: coalescing
// pre-empts every refusal gate, so a coalesced request would never
// reach resolveCausation and would defeat this test's row count).
func TestQueueRun_RefusesSelfTriggerBeyondAllowance(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetLaunchSafetyLimits(runsservice.DefaultMaxCausationDepth, 2, runsservice.DefaultSelfTriggerTotalAllowance)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
			AgentProfileID: "agent-primary",
			Reason:         "self_trigger_reason",
			ActorKind:      models.ActorKindAgent,
			ActorID:        "agent-primary",
			IdempotencyKey: fmt.Sprintf("task_comment:self-trigger-%d", i),
		}); err != nil {
			t.Fatalf("queue self-trigger %d: %v", i, err)
		}
	}

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "self_trigger_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:self-trigger-third",
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue third self-trigger: err = %v, want *RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalSelfTrigger {
		t.Fatalf("refusal gate = %q, want %q", refusal.Gate, runsservice.RefusalSelfTrigger)
	}
}

// TestQueueRun_MissingActorNormalizesToSystem pins
// AC-OFFICE-RUN-CAUSATION-001.16: an unset ActorKind resolves to
// ActorKindSystem rather than defaulting to a human actor.
func TestQueueRun_MissingActorNormalizesToSystem(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "no_actor_declared",
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run := getRun(t, repo, "agent-primary", "no_actor_declared")
	if run.ActorKind != models.ActorKindSystem {
		t.Fatalf("actor_kind = %q, want %q", run.ActorKind, models.ActorKindSystem)
	}
	if run.HumanRooted {
		t.Fatalf("human_rooted = true, want false for a system-defaulted actor")
	}
}
