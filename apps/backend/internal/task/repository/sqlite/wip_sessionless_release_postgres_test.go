package sqlite

// PostgreSQL parity for the opt-in sessionless WIP release: the shared release
// predicate inside every admitted count, the locked queue promotion, and the
// occupancy read transaction (repeatable read on PostgreSQL).
// Skips unless KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newPostgresReleaseFixture(t *testing.T, enabled bool) *releaseFixture {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	return newReleaseFixtureWithRepo(t, repo, enabled)
}

func TestPostgresWIPSessionlessReleasePromotesAfterGrace(t *testing.T) {
	off := newPostgresReleaseFixture(t, false)
	off.occupant("off-failed", v1.TaskStateFailed, time.Hour)
	if got := off.counted(); got != 1 {
		t.Fatalf("policy off counted = %d, want 1", got)
	}
	if off.promote(off.queued("off-queued")) {
		t.Fatal("policy off promoted into a full step")
	}

	f := newPostgresReleaseFixture(t, true)
	f.occupant("stuck", v1.TaskStateFailed, time.Hour)
	f.session("stuck-cancelled", "stuck", models.TaskSessionStateCancelled, 30*time.Minute)
	steps, err := f.repo.ListSessionlessWIPReleaseSteps(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0] != releaseStepID {
		t.Fatalf("release steps = %v, want [%s]", steps, releaseStepID)
	}
	if got := f.counted(); got != 0 {
		t.Fatalf("counted = %d, want 0 once the grace passed", got)
	}
	if !f.promote(f.queued("queued-next")) {
		t.Fatal("queued task was not promoted into the released slot")
	}
	stuck, err := f.repo.GetTask(f.ctx, "stuck")
	if err != nil {
		t.Fatal(err)
	}
	if stuck.State != v1.TaskStateFailed || stuck.WorkflowStepID != releaseStepID || !stuck.WIPAdmitted {
		t.Fatalf("released task was changed: %+v", stuck)
	}
	if got := f.counted(); got != 1 {
		t.Fatalf("counted after pull = %d, want 1", got)
	}
}

func TestPostgresWIPSessionlessReleaseHoldsForGraceAndLiveSessions(t *testing.T) {
	f := newPostgresReleaseFixture(t, true)
	f.occupant("recent", v1.TaskStateFailed, time.Hour)
	f.session("recent-failed", "recent", models.TaskSessionStateFailed, 2*time.Minute)
	if got := f.counted(); got != 1 {
		t.Fatalf("counted = %d, want 1 inside the grace", got)
	}

	g := newPostgresReleaseFixture(t, true)
	g.occupant("returning", v1.TaskStateInProgress, time.Hour)
	if got := g.counted(); got != 0 {
		t.Fatalf("counted = %d, want 0 for a sessionless task past its grace", got)
	}
	g.session("returning-new", "returning", models.TaskSessionStateRunning, 0)
	if got := g.counted(); got != 1 {
		t.Fatalf("counted = %d, want 1 after the task re-entered a live session", got)
	}
	if g.promote(g.queued("queued-next")) {
		t.Fatal("queued task was promoted while the re-entered task holds the slot")
	}
}

func TestPostgresWIPStepOccupancyMatchesAdmission(t *testing.T) {
	f := newPostgresReleaseFixture(t, true)
	f.occupant("a-live", v1.TaskStateInProgress, time.Hour)
	f.session("a-live-running", "a-live", models.TaskSessionStateRunning, time.Hour)
	f.occupant("b-released", v1.TaskStateFailed, time.Hour)
	f.occupant("c-pending", v1.TaskStateFailed, time.Hour)
	f.session("c-pending-failed", "c-pending", models.TaskSessionStateFailed, 3*time.Minute)

	occupancy, err := f.repo.GetWorkflowStepWIPOccupancy(f.ctx, releaseStepID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]models.WIPOccupantReason{
		"a-live":     models.WIPOccupantReasonLiveSession,
		"b-released": models.WIPOccupantReasonReleasedNoLiveSession,
		"c-pending":  models.WIPOccupantReasonReleaseGracePending,
	}
	if len(occupancy.Occupants) != len(want) {
		t.Fatalf("occupants = %+v", occupancy.Occupants)
	}
	for _, occupant := range occupancy.Occupants {
		if occupant.Reason != want[occupant.TaskID] {
			t.Errorf("%s reason = %s, want %s", occupant.TaskID, occupant.Reason, want[occupant.TaskID])
		}
	}
	if got := f.counted(); occupancy.Counted != got || got != 2 {
		t.Fatalf("occupancy counted = %d, admission counted = %d, want 2", occupancy.Counted, got)
	}
}
