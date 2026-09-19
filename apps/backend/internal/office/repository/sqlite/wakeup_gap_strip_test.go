package sqlite_test

import (
	"context"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// mergeEntryPoint abstracts over the two coalesce-merge call sites so the
// table test below exercises both with identical fixtures and assertions
// (docs/specs/office/system-design/routine-catch-up-02.md "Testing": "Every
// case below runs against BOTH merge entry points ... That is not
// belt-and-braces: until the refactor lands they are two separate SQL
// statements, and a suite that exercises only the helper passes green with
// the promoted-run path fully broken").
type mergeEntryPoint struct {
	name  string
	merge func(t *testing.T, repo *sqlite.Repository, ctx context.Context, requestID, runID string)
}

var mergeEntryPoints = []mergeEntryPoint{
	{
		name: "MarkWakeupRequestCoalesced",
		merge: func(t *testing.T, repo *sqlite.Repository, ctx context.Context, requestID, runID string) {
			t.Helper()
			if err := repo.MarkWakeupRequestCoalesced(ctx, requestID, runID); err != nil {
				t.Fatalf("MarkWakeupRequestCoalesced: %v", err)
			}
		},
	},
	{
		name: "PromoteRunAndCoalesceWakeupIfQueued",
		merge: func(t *testing.T, repo *sqlite.Repository, ctx context.Context, requestID, runID string) {
			t.Helper()
			ok, err := repo.PromoteRunAndCoalesceWakeupIfQueued(ctx, requestID, runID, shared.RunReasonRoutineDispatchEvent)
			if err != nil {
				t.Fatalf("PromoteRunAndCoalesceWakeupIfQueued: %v", err)
			}
			if !ok {
				t.Fatal("PromoteRunAndCoalesceWakeupIfQueued: expected queued run to accept the promote")
			}
		},
	},
}

// seedGapStripRun creates a queued run with the given snapshot, ready for
// either merge entry point (PromoteRunAndCoalesceWakeupIfQueued requires
// status "queued").
func seedGapStripRun(t *testing.T, repo *sqlite.Repository, ctx context.Context, id, snapshot string) {
	t.Helper()
	run := &officemodels.Run{
		ID: id, AgentProfileID: "agent-1", Reason: shared.RunReasonRoutineDispatchCron,
		Payload: "{}", Status: "queued", CoalescedCount: 1, ContextSnapshot: snapshot,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
}

// TestMergeEntryPoints_GapCarryingWakeIntoGaplessRun covers the direction
// AC-002.10 forbids: a gap-carrying routine wake coalescing into an
// in-flight gapless run must leave that run gapless.
func TestMergeEntryPoints_GapCarryingWakeIntoGaplessRun(t *testing.T) {
	for _, ep := range mergeEntryPoints {
		t.Run(ep.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()
			seedGapStripRun(t, repo, ctx, "run-gapless-"+ep.name, `{"routine_id":"r-1"}`)

			reqID := "req-gap-" + ep.name
			if err := repo.CreateWakeupRequest(ctx, &sqlite.WakeupRequest{
				ID: reqID, AgentProfileID: "agent-1", Source: "routine",
				Payload: `{"routine_id":"r-1","missed_ticks":5,"missed_since":"2026-05-10T11:55:00Z","missed_truncated":true}`,
			}); err != nil {
				t.Fatalf("create wakeup: %v", err)
			}

			ep.merge(t, repo, ctx, reqID, "run-gapless-"+ep.name)

			gotRun, err := repo.GetRunByID(ctx, "run-gapless-"+ep.name)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			for _, key := range []string{"missed_ticks", "missed_since", "missed_truncated"} {
				if contains(gotRun.ContextSnapshot, key) {
					t.Errorf("context_snapshot = %q, must not carry %q from a coalesced wake", gotRun.ContextSnapshot, key)
				}
			}
			if !contains(gotRun.ContextSnapshot, `"routine_id":"r-1"`) {
				t.Errorf("context_snapshot = %q, want routine_id preserved", gotRun.ContextSnapshot)
			}
		})
	}
}

// TestMergeEntryPoints_GaplessWakeIntoGapCarryingRun covers the other
// direction: a gapless wake coalescing into a run that already carries a
// gap must leave the original gap intact, not half-overwritten.
func TestMergeEntryPoints_GaplessWakeIntoGapCarryingRun(t *testing.T) {
	for _, ep := range mergeEntryPoints {
		t.Run(ep.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()
			seedGapStripRun(t, repo, ctx, "run-gap-"+ep.name,
				`{"routine_id":"r-1","missed_ticks":5,"missed_since":"2026-05-10T11:55:00Z","missed_truncated":true}`)

			reqID := "req-nogap-" + ep.name
			if err := repo.CreateWakeupRequest(ctx, &sqlite.WakeupRequest{
				ID: reqID, AgentProfileID: "agent-1", Source: "routine",
				Payload: `{"routine_id":"r-1"}`,
			}); err != nil {
				t.Fatalf("create wakeup: %v", err)
			}

			ep.merge(t, repo, ctx, reqID, "run-gap-"+ep.name)

			gotRun, err := repo.GetRunByID(ctx, "run-gap-"+ep.name)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !contains(gotRun.ContextSnapshot, `"missed_ticks":5`) {
				t.Errorf("context_snapshot = %q, want the original gap statement intact", gotRun.ContextSnapshot)
			}
			if !contains(gotRun.ContextSnapshot, `"missed_truncated":true`) {
				t.Errorf("context_snapshot = %q, want the original truncated flag intact", gotRun.ContextSnapshot)
			}
		})
	}
}

// TestMergeEntryPoints_NonRoutinePayloadRoundTripsUnchanged proves the
// json_remove strip is safe for a source that never writes the three
// catch-up keys: the merge is byte-for-byte unaffected.
func TestMergeEntryPoints_NonRoutinePayloadRoundTripsUnchanged(t *testing.T) {
	for _, ep := range mergeEntryPoints {
		t.Run(ep.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()
			seedGapStripRun(t, repo, ctx, "run-nonroutine-"+ep.name, `{"existing":"old"}`)

			reqID := "req-comment-" + ep.name
			if err := repo.CreateWakeupRequest(ctx, &sqlite.WakeupRequest{
				ID: reqID, AgentProfileID: "agent-1", Source: "comment",
				Payload: `{"task_id":"t-1","comment_id":"c-1"}`,
			}); err != nil {
				t.Fatalf("create wakeup: %v", err)
			}

			ep.merge(t, repo, ctx, reqID, "run-nonroutine-"+ep.name)

			gotRun, err := repo.GetRunByID(ctx, "run-nonroutine-"+ep.name)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !contains(gotRun.ContextSnapshot, `"task_id":"t-1"`) || !contains(gotRun.ContextSnapshot, `"comment_id":"c-1"`) {
				t.Errorf("context_snapshot = %q, want the non-routine payload merged unchanged", gotRun.ContextSnapshot)
			}
		})
	}
}
