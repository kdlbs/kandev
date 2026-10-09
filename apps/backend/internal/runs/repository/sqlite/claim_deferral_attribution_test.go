package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"expvar"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/shared"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// deferredTotalForGate reads office_launch_deferred_total's current count
// for gate, treating an unset label as 0 rather than a lookup error.
func deferredTotalForGate(t *testing.T, gate string) int64 {
	t.Helper()
	v := shared.LaunchDeferredTotal.Get(shared.LaunchSafetyLabel("gate", gate))
	if v == nil {
		return 0
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		t.Fatalf("office_launch_deferred_total[%s] is %T, want *expvar.Int", gate, v)
	}
	return iv.Value()
}

// TestClaimNextEligibleRun_DeferralAttributedOncePerAttempt pins
// AC-OFFICE-BACKPRESSURE-003.6/.7: a claim attempt that blocks on
// several candidates before returning no row increments the deferred
// counter exactly once, attributed to the highest-priority (first,
// under the effective claim order) candidate — not once per blocked
// candidate, which is the "counting per blocked run is explicitly not
// required" the requirement rules out.
func TestClaimNextEligibleRun_DeferralAttributedOncePerAttempt(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// Four different agents sharing one workspace at MaxConcurrentWorkspace=1.
	// d0 fills the single slot first; d1/d2/d3 then all queue behind it, so
	// every one of them blocks on the shared workspace ceiling instead of
	// their own (generous) per-agent ceiling — the broader gate that lets
	// several distinct candidates all be blocked by the same attempt.
	const sharedWorkspace = "shared-ws-deferral"
	seedAgentInWorkspace(t, repo, "d0", sharedWorkspace, 5)
	seedAgentInWorkspace(t, repo, "d1", sharedWorkspace, 5)
	seedAgentInWorkspace(t, repo, "d2", sharedWorkspace, 5)
	seedAgentInWorkspace(t, repo, "d3", sharedWorkspace, 5)
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{MaxConcurrentWorkspace: 1})

	queueRunInWorkspace(t, repo, "d-zeroth", "d0", sharedWorkspace, base.Add(-time.Hour))
	if _, err := repo.ClaimNextEligibleRun(ctx); err != nil {
		t.Fatalf("fill the workspace slot: %v", err)
	}

	queueRunInWorkspace(t, repo, "d-first", "d1", sharedWorkspace, base)
	queueRunInWorkspace(t, repo, "d-second", "d2", sharedWorkspace, base.Add(time.Minute))
	queueRunInWorkspace(t, repo, "d-third", "d3", sharedWorkspace, base.Add(2*time.Minute))

	before := deferredTotalForGate(t, "workspace_ceiling")

	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claim err = %v, want sql.ErrNoRows", err)
	}

	after := deferredTotalForGate(t, "workspace_ceiling")
	if after-before != 1 {
		t.Errorf("office_launch_deferred_total[workspace_ceiling] increased by %d, want 1 (once per attempt, not per blocked candidate)", after-before)
	}
}
