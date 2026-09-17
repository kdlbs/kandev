package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestCountAgentInitiatedAssignmentWakes_BasicCount covers
// AC-OFFICE-ASSIGN-RATE-001.2: only rows matching reason, actor_type
// "agent", the target task, and the window count.
func TestCountAgentInitiatedAssignmentWakes_BasicCount(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	windowStart := now.Add(-10 * time.Minute)

	inWindow := func(id, payload string) {
		r := mustCreateRun(t, repo, &models.Run{
			ID: id, AgentProfileID: "a1", Reason: "task_assigned",
			Payload: payload, Status: models.RunStatusQueued, CoalescedCount: 1,
		})
		setRequestedAt(t, repo, r.ID, now)
	}

	inWindow("cnt-agent-1", `{"task_id":"t1","actor_type":"agent"}`)
	inWindow("cnt-agent-2", `{"task_id":"t1","actor_type":"agent"}`)
	inWindow("cnt-user", `{"task_id":"t1","actor_type":"user"}`)
	inWindow("cnt-other-task", `{"task_id":"t2","actor_type":"agent"}`)

	other := mustCreateRun(t, repo, &models.Run{
		ID: "cnt-other-reason", AgentProfileID: "a1", Reason: "task_comment",
		Payload: `{"task_id":"t1","actor_type":"agent"}`, Status: models.RunStatusQueued, CoalescedCount: 1,
	})
	setRequestedAt(t, repo, other.ID, now)

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "t1", "task_assigned", windowStart, now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

// TestCountAgentInitiatedAssignmentWakes_WindowIsHalfOpenExclusiveStart
// covers AC-OFFICE-ASSIGN-RATE-001.11: a row whose requested_at is exactly
// windowStart is one window old and must not count; a row one instant
// later must.
func TestCountAgentInitiatedAssignmentWakes_WindowIsHalfOpenExclusiveStart(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	windowStart := time.Now().UTC()

	atBoundary := mustCreateRun(t, repo, &models.Run{
		ID: "win-at-boundary", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"t1","actor_type":"agent"}`, Status: models.RunStatusQueued, CoalescedCount: 1,
	})
	setRequestedAt(t, repo, atBoundary.ID, windowStart)

	insideWindow := mustCreateRun(t, repo, &models.Run{
		ID: "win-inside", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"t1","actor_type":"agent"}`, Status: models.RunStatusQueued, CoalescedCount: 1,
	})
	setRequestedAt(t, repo, insideWindow.ID, windowStart.Add(time.Millisecond))

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "t1", "task_assigned", windowStart, windowStart.Add(time.Millisecond))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (only the row strictly after windowStart)", count)
	}
}

// TestCountAgentInitiatedAssignmentWakes_WindowIsHalfOpenInclusiveEnd covers
// AC-OFFICE-ASSIGN-RATE-001.11's other edge: a row whose requested_at is
// exactly the evaluation instant is inside the window and must count; a row
// strictly after the evaluation instant (a future timestamp, reachable under
// clock skew) is outside it and must not.
func TestCountAgentInitiatedAssignmentWakes_WindowIsHalfOpenInclusiveEnd(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	evaluationInstant := time.Now().UTC()
	windowStart := evaluationInstant.Add(-10 * time.Minute)

	atBoundary := mustCreateRun(t, repo, &models.Run{
		ID: "win-end-at-boundary", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"t-end","actor_type":"agent"}`, Status: models.RunStatusQueued, CoalescedCount: 1,
	})
	setRequestedAt(t, repo, atBoundary.ID, evaluationInstant)

	future := mustCreateRun(t, repo, &models.Run{
		ID: "win-end-future", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"t-end","actor_type":"agent"}`, Status: models.RunStatusQueued, CoalescedCount: 1,
	})
	setRequestedAt(t, repo, future.ID, evaluationInstant.Add(time.Millisecond))

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "t-end", "task_assigned", windowStart, evaluationInstant)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (the at-boundary row counts, the future row does not)", count)
	}
}

// TestCountAgentInitiatedAssignmentWakes_GuardsNonStringTaskID covers the
// dialect.JSONTypeIsString guard the design's prose directs (the SQL
// sketch omits it): a stored numeric task_id must not textually match an
// incoming string task_id and silently join another task's allowance.
func TestCountAgentInitiatedAssignmentWakes_GuardsNonStringTaskID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	r := mustCreateRun(t, repo, &models.Run{
		ID: "guard-numeric", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":42,"actor_type":"agent"}`, Status: models.RunStatusQueued, CoalescedCount: 1,
	})
	setRequestedAt(t, repo, r.ID, now)

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "42", "task_assigned", now.Add(-time.Minute), now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 (a numeric stored task_id must not match a string taskID)", count)
	}
}

// TestCountAgentInitiatedAssignmentWakes_NoStatusFilter covers
// AC-OFFICE-ASSIGN-RATE-001.4's deliberate omission: "admitted" means "a
// runs row was inserted", full stop, so a run that has since completed
// still holds its allowance slot until the window passes.
func TestCountAgentInitiatedAssignmentWakes_NoStatusFilter(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, tc := range []struct {
		id     string
		status models.RunStatus
	}{
		{"status-queued", models.RunStatusQueued},
		{"status-claimed", models.RunStatusClaimed},
		{"status-finished", models.RunStatusFinished},
		{"status-failed", models.RunStatusFailed},
	} {
		r := mustCreateRun(t, repo, &models.Run{
			ID: tc.id, AgentProfileID: "a1", Reason: "task_assigned",
			Payload: `{"task_id":"t-status","actor_type":"agent"}`,
			Status:  tc.status, CoalescedCount: 1,
		})
		setRequestedAt(t, repo, r.ID, now)
	}

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "t-status", "task_assigned", now.Add(-time.Minute), now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 4 {
		t.Fatalf("count = %d, want 4 (every status counts)", count)
	}
}

// TestCountAgentInitiatedAssignmentWakes_TasklessAndAbsentActorDoNotMatch
// pins that a taskless or actor-less stored row cannot accidentally
// satisfy the count query for a real task/actor pair.
func TestCountAgentInitiatedAssignmentWakes_TasklessAndAbsentActorDoNotMatch(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	taskless := mustCreateRun(t, repo, &models.Run{
		ID: "taskless", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"actor_type":"agent"}`, Status: models.RunStatusQueued, CoalescedCount: 1,
	})
	setRequestedAt(t, repo, taskless.ID, now)

	noActor := mustCreateRun(t, repo, &models.Run{
		ID: "no-actor", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"t1"}`, Status: models.RunStatusQueued, CoalescedCount: 1,
	})
	setRequestedAt(t, repo, noActor.ID, now)

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "t1", "task_assigned", now.Add(-time.Minute), now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}
