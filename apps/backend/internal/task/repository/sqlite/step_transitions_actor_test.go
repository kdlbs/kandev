package sqlite

// Actor and privacy coverage: the synthetic auth-disabled identity records as
// human, a watcher-originated genesis row records as integration with the
// watch ID, and no text column of any row leaks a display name, email,
// title, or prompt across the full trigger matrix.

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
)

func TestGenesisRowSyntheticIdentityRecordsHumanWithUserID(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "synthetic-user", Role: authn.RoleAdmin, Synthetic: true,
	})
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-synthetic-actor", "wf-1", "step-a")

	rows := stepTransitionRowsForTask(t, repo, "task-synthetic-actor")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].actorKind != string(steptelemetry.ActorHuman) {
		t.Fatalf("actor_kind = %q, want %q", rows[0].actorKind, steptelemetry.ActorHuman)
	}
	if rows[0].actorID == nil || *rows[0].actorID != "synthetic-user" {
		t.Fatalf("actor_id = %v, want synthetic-user", rows[0].actorID)
	}
}

func TestGenesisRowIntegrationActorRecordsWatchID(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		ActorKind: steptelemetry.ActorIntegration, ActorID: "jira-watch-42",
	})
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-integration-actor", "wf-1", "step-a")

	rows := stepTransitionRowsForTask(t, repo, "task-integration-actor")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].actorKind != string(steptelemetry.ActorIntegration) {
		t.Fatalf("actor_kind = %q, want %q", rows[0].actorKind, steptelemetry.ActorIntegration)
	}
	if rows[0].actorID == nil || *rows[0].actorID != "jira-watch-42" {
		t.Fatalf("actor_id = %v, want jira-watch-42", rows[0].actorID)
	}
	if rows[0].trigger != string(steptelemetry.TriggerTaskCreated) {
		t.Fatalf("trigger = %q, want %q (genesis is always hard-coded)", rows[0].trigger, steptelemetry.TriggerTaskCreated)
	}
}

// TestGenesisRowAgentActorRecordsSessionID closes the should-fix gap Review
// round 3 found: hardcodedTriggerAttribution's preset branch forwarded
// ActorKind/ActorID but not SessionID, unlike workflowAttachedAttribution
// (its own doc comment claims to mirror). This was dormant when found (no
// caller set a preset with SessionID through this path) but went live the
// moment CreateChildTaskCallback.Execute (must-fix #1, this same round)
// started presetting ActorAgent+SessionID before task creation — without
// this fix, a session-caused child task's genesis row would carry the
// right actor_id but a NULL session_id.
func TestGenesisRowAgentActorRecordsSessionID(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "parent-task", "wf-1", "step-a")
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "causal-sess-1", TaskID: "parent-task", State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		ActorKind: steptelemetry.ActorAgent, ActorID: "causal-sess-1", SessionID: "causal-sess-1",
	})
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-genesis-agent", "wf-1", "step-a")

	rows := stepTransitionRowsForTask(t, repo, "task-genesis-agent")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].sessionID == nil || *rows[0].sessionID != "causal-sess-1" {
		t.Fatalf("session_id = %v, want causal-sess-1", rows[0].sessionID)
	}
}

// TestNoSensitiveTextLeaksIntoNonActorColumns scans every text column of
// rows produced against a fixture whose title/description carry a PII-shaped
// sentinel (display-name-and-email-shaped, distinct from the actor_id
// sentinel below) that production code never derives actor_id from and must
// never copy anywhere. Review round 4 found the prior version of this test
// tautological: the sentinel was injected only as actor_id and the fixture's
// title/description were the unrelated hardcoded literal "Test Task", so the
// scanned columns (from_*/to_*/session_id/trigger/actor_kind) could never
// contain it regardless of whether a real leak existed — the test passed by
// construction. This version seeds the PII sentinel into fields a bug could
// plausibly copy from (title, description) and scans EVERY text column,
// including actor_id, for it.
func TestNoSensitiveTextLeaksIntoNonActorColumns(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	const piiSentinel = "SENTINEL-Jane-Doe-jane@example.com-please-do-the-thing"
	const actorSentinel = "SENTINEL-ACTOR-9f3c2b1a-opaque-identifier"

	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerManualMove, ActorKind: steptelemetry.ActorHuman, ActorID: actorSentinel,
	})
	task := &models.Task{
		ID: "task-privacy", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-a",
		Title: piiSentinel, Description: piiSentinel, Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-privacy")
	if len(rows) == 0 {
		t.Fatal("no ledger rows for task-privacy")
	}
	for _, row := range rows {
		for _, col := range []*string{
			row.fromWorkflowID, row.fromWorkflowStepID, row.toWorkflowID, row.toWorkflowStepID,
			row.sessionID, row.actorID,
		} {
			if col != nil && strings.Contains(*col, piiSentinel) {
				t.Fatalf("PII sentinel leaked into a ledger column: %q", *col)
			}
		}
		if strings.Contains(row.trigger, piiSentinel) {
			t.Fatalf("PII sentinel leaked into trigger column: %q", row.trigger)
		}
		if strings.Contains(row.actorKind, piiSentinel) {
			t.Fatalf("PII sentinel leaked into actor_kind column: %q", row.actorKind)
		}
	}

	// Separate positive assertion: a legitimate opaque caller-supplied
	// actor_id round-trips unmodified (not truncated, not hashed) — this
	// keeps the leak-detection loop above honest by proving actor_id really
	// is populated and compared, not vacuously empty or always absent.
	last := rows[len(rows)-1]
	if last.actorID == nil || *last.actorID != actorSentinel {
		t.Fatalf("actor_id = %v, want the caller-supplied identifier %q", last.actorID, actorSentinel)
	}
}
