package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestPostgresCountAgentInitiatedAssignmentWakes_BasicCount is the
// Postgres twin of TestCountAgentInitiatedAssignmentWakes_BasicCount.
// CountAgentInitiatedAssignmentWakes is built from dialect.JSONExtract,
// which emits payload::jsonb->>'task_id' on Postgres versus
// json_extract(payload, '$.task_id') on SQLite — a passing SQLite
// assertion is not evidence the Postgres fragment even parses. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresCountAgentInitiatedAssignmentWakes_BasicCount(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()
	now := time.Now().UTC()
	windowStart := now.Add(-10 * time.Minute)

	inWindow := func(id, payload string) {
		r := mustCreateRun(t, repo, &models.Run{
			ID: id, AgentProfileID: "a1", Reason: "task_assigned",
			Payload: payload, Status: "queued", CoalescedCount: 1,
		})
		setRequestedAt(t, repo, r.ID, now)
	}

	inWindow("pg-cnt-agent-1", `{"task_id":"pg-t1","actor_type":"agent"}`)
	inWindow("pg-cnt-agent-2", `{"task_id":"pg-t1","actor_type":"agent"}`)
	inWindow("pg-cnt-user", `{"task_id":"pg-t1","actor_type":"user"}`)
	inWindow("pg-cnt-other-task", `{"task_id":"pg-t2","actor_type":"agent"}`)

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "pg-t1", "task_assigned", windowStart, now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

// TestPostgresCountAgentInitiatedAssignmentWakes_GuardsNonStringTaskID is
// the Postgres twin of the non-string task_id guard test: Postgres's ->>
// operator converts a stored JSON number to text before comparison, so a
// naive text-only predicate would let a string-valued taskID match a
// stored {"task_id":42} and silently join another task's allowance. This
// is the exact dialect coercion dialect.JSONTypeIsString exists to guard
// against.
func TestPostgresCountAgentInitiatedAssignmentWakes_GuardsNonStringTaskID(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()
	now := time.Now().UTC()

	r := mustCreateRun(t, repo, &models.Run{
		ID: "pg-guard-numeric", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":42,"actor_type":"agent"}`, Status: "queued", CoalescedCount: 1,
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

// TestPostgresCountAgentInitiatedAssignmentWakes_WindowIsHalfOpenExclusiveStart
// is the Postgres twin of the window-boundary test.
func TestPostgresCountAgentInitiatedAssignmentWakes_WindowIsHalfOpenExclusiveStart(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()
	windowStart := time.Now().UTC()

	atBoundary := mustCreateRun(t, repo, &models.Run{
		ID: "pg-win-at-boundary", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"pg-t3","actor_type":"agent"}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, atBoundary.ID, windowStart)

	insideWindow := mustCreateRun(t, repo, &models.Run{
		ID: "pg-win-inside", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"pg-t3","actor_type":"agent"}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, insideWindow.ID, windowStart.Add(time.Millisecond))

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "pg-t3", "task_assigned", windowStart, windowStart.Add(time.Millisecond))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (only the row strictly after windowStart)", count)
	}
}

// TestPostgresCountAgentInitiatedAssignmentWakes_WindowIsHalfOpenInclusiveEnd
// is the Postgres twin of the window's upper-bound test: a row at the
// evaluation instant counts, a row strictly after it does not.
func TestPostgresCountAgentInitiatedAssignmentWakes_WindowIsHalfOpenInclusiveEnd(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()
	evaluationInstant := time.Now().UTC()
	windowStart := evaluationInstant.Add(-10 * time.Minute)

	atBoundary := mustCreateRun(t, repo, &models.Run{
		ID: "pg-win-end-at-boundary", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"pg-t4","actor_type":"agent"}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, atBoundary.ID, evaluationInstant)

	future := mustCreateRun(t, repo, &models.Run{
		ID: "pg-win-end-future", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"pg-t4","actor_type":"agent"}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, future.ID, evaluationInstant.Add(time.Millisecond))

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, "pg-t4", "task_assigned", windowStart, evaluationInstant)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (the at-boundary row counts, the future row does not)", count)
	}
}
