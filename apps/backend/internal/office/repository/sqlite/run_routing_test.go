package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newTestRepoWithDB returns both the repo and the underlying *sqlx.DB
// so tests can seed agent_profiles rows directly. The base newTestRepo
// helper hides the DB handle; ClearAllParkedRoutingForWorkspace joins
// runs ⨝ agent_profiles, so the SQL-level test needs row-level access.
func newTestRepoWithDB(t *testing.T) (*sqlite.Repository, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo, db
}

// seedAgentProfile inserts a minimal agent_profiles row so a run can
// JOIN through to a workspace_id. Mirrors the production schema (see
// settings/store/sqlite.go) without requiring the full settings API.
func seedAgentProfile(t *testing.T, db *sqlx.DB, id, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, workspace_id, role,
		 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
		id, "agent", "name", "display", workspaceID, now, now)
	if err != nil {
		t.Fatalf("seed agent_profile: %v", err)
	}
}

func TestClearAllParkedRoutingForWorkspace_ClearsOnlyTargetWorkspace(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	// Two workspaces, one parked run each.
	seedAgentProfile(t, db, "agent-a", "ws-a")
	seedAgentProfile(t, db, "agent-b", "ws-b")

	runA := &models.Run{
		AgentProfileID: "agent-a",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	runB := &models.Run{
		AgentProfileID: "agent-b",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t2"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, runA); err != nil {
		t.Fatalf("create runA: %v", err)
	}
	if err := repo.CreateRun(ctx, runB); err != nil {
		t.Fatalf("create runB: %v", err)
	}

	// Park both runs under waiting_for_provider_capacity. Use distinct
	// future timestamps to verify both columns reset to NULL.
	retry := time.Now().UTC().Add(10 * time.Minute)
	if err := repo.ParkRunForProviderCapacity(ctx, runA.ID, "waiting_for_provider_capacity", retry); err != nil {
		t.Fatalf("park runA: %v", err)
	}
	if err := repo.ParkRunForProviderCapacity(ctx, runB.ID, "blocked_provider_action_required", retry); err != nil {
		t.Fatalf("park runB: %v", err)
	}

	// Clear ws-a only.
	if err := repo.ClearAllParkedRoutingForWorkspace(ctx, "ws-a"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	gotA, err := repo.GetRunByID(ctx, runA.ID)
	if err != nil {
		t.Fatalf("get runA: %v", err)
	}
	gotB, err := repo.GetRunByID(ctx, runB.ID)
	if err != nil {
		t.Fatalf("get runB: %v", err)
	}

	if gotA.RoutingBlockedStatus != nil && *gotA.RoutingBlockedStatus != "" {
		t.Errorf("ws-a run still parked: routing_blocked_status=%q",
			derefStr(gotA.RoutingBlockedStatus))
	}
	if gotA.EarliestRetryAt != nil {
		t.Errorf("ws-a earliest_retry_at not cleared: %v", gotA.EarliestRetryAt)
	}
	if gotA.ScheduledRetryAt != nil {
		t.Errorf("ws-a scheduled_retry_at not cleared: %v", gotA.ScheduledRetryAt)
	}
	if gotA.Status != "queued" {
		t.Errorf("ws-a status = %q, want queued", gotA.Status)
	}

	// ws-b must be untouched.
	if gotB.RoutingBlockedStatus == nil || *gotB.RoutingBlockedStatus != "blocked_provider_action_required" {
		t.Errorf("ws-b run incorrectly cleared: routing_blocked_status=%q",
			derefStr(gotB.RoutingBlockedStatus))
	}
	if gotB.EarliestRetryAt == nil {
		t.Error("ws-b earliest_retry_at unexpectedly cleared")
	}
}

func TestClearAllParkedRoutingForWorkspace_NoParkedRuns(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedAgentProfile(t, db, "agent-a", "ws-a")
	run := &models.Run{
		AgentProfileID: "agent-a",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.ClearAllParkedRoutingForWorkspace(ctx, "ws-a"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ := repo.GetRunByID(ctx, run.ID)
	if got.Status != "queued" {
		t.Errorf("status changed unexpectedly: %q", got.Status)
	}
}

// TestListRunsWaitingOnProvider_MatchesParkedRunForProvider is the
// happy-path sanity check that the LIKE join over logical_provider_order
// still finds parked runs after the wildcard escape was added — without
// this, a regression to likeEscaper that escapes the JSON-quote literal
// would silently match nothing.
func TestListRunsWaitingOnProvider_MatchesParkedRunForProvider(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedAgentProfile(t, db, "agent-a", "ws-a")
	run := &models.Run{
		AgentProfileID: "agent-a",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.SetRunRoutingDecision(ctx, run.ID,
		`["claude-acp","codex-acp"]`, "balanced"); err != nil {
		t.Fatalf("set decision: %v", err)
	}
	if err := repo.ParkRunForProviderCapacity(ctx, run.ID,
		"waiting_for_provider_capacity", time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatalf("park: %v", err)
	}

	got, err := repo.ListRunsWaitingOnProvider(ctx, "ws-a", "claude-acp")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != run.ID {
		t.Fatalf("want [%s], got %v", run.ID, got)
	}
}

// TestListRunsWaitingOnProvider_WildcardProviderIDMatchesNone pins the
// likeEscaper fix for Round 2 Blocker 6. A providerID of "%" must not
// match every parked run in the workspace — without escaping, the
// wildcard substring would short-circuit the JSON-token match and
// silently re-dispatch every blocked run on a single provider recovery.
func TestListRunsWaitingOnProvider_WildcardProviderIDMatchesNone(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedAgentProfile(t, db, "agent-a", "ws-a")
	run := &models.Run{
		AgentProfileID: "agent-a",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.SetRunRoutingDecision(ctx, run.ID,
		`["claude-acp"]`, "balanced"); err != nil {
		t.Fatalf("set decision: %v", err)
	}
	if err := repo.ParkRunForProviderCapacity(ctx, run.ID,
		"waiting_for_provider_capacity", time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatalf("park: %v", err)
	}

	got, err := repo.ListRunsWaitingOnProvider(ctx, "ws-a", "%")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("wildcard providerID should match nothing; got %d runs", len(got))
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// TestClaimNextEligibleRun_SkipsRoutingBlocked verifies the eligibility
// filter excludes runs parked under any routing_blocked_status. Without
// this filter, blocked_provider_action_required runs (which have no
// scheduled_retry_at set) would be re-claimed every tick and immediately
// re-parked, churning the scheduler.
func TestClaimNextEligibleRun_SkipsRoutingBlocked(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedAgentProfile(t, db, "agent-a", "ws-a")
	run := &models.Run{
		AgentProfileID: "agent-a",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Park under blocked_provider_action_required (no retry time).
	if err := repo.ParkRunForProviderCapacity(ctx, run.ID,
		"blocked_provider_action_required", time.Time{}); err != nil {
		t.Fatalf("park: %v", err)
	}
	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err == nil && claimed != nil {
		t.Fatalf("expected no claimable run while routing-blocked; got %s", claimed.ID)
	}

	// Clear the routing block and verify the run is now claimable.
	if err := repo.ClearRoutingBlock(ctx, run.ID); err != nil {
		t.Fatalf("clear block: %v", err)
	}
	claimed, err = repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim after clear: %v", err)
	}
	if claimed == nil || claimed.ID != run.ID {
		t.Fatalf("expected run claimable after ClearRoutingBlock; got %v", claimed)
	}
}

// TestClearRoutingBlock_ClearsScheduledRetryAt verifies that the
// per-run wake/retry path doesn't leave scheduled_retry_at set. Before
// this fix, "Retry now" would clear routing_blocked_status but leave
// scheduled_retry_at pointing to the original retry time, so the
// run would be skipped by the eligibility filter until that time.
func TestClearRoutingBlock_ClearsScheduledRetryAt(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedAgentProfile(t, db, "agent-a", "ws-a")
	run := &models.Run{
		AgentProfileID: "agent-a",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create: %v", err)
	}
	future := time.Now().UTC().Add(10 * time.Minute)
	if err := repo.ParkRunForProviderCapacity(ctx, run.ID,
		"waiting_for_provider_capacity", future); err != nil {
		t.Fatalf("park: %v", err)
	}
	if err := repo.ClearRoutingBlock(ctx, run.ID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err := repo.GetRunByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ScheduledRetryAt != nil {
		t.Errorf("scheduled_retry_at not cleared: %v", got.ScheduledRetryAt)
	}
	if got.EarliestRetryAt != nil {
		t.Errorf("earliest_retry_at not cleared: %v", got.EarliestRetryAt)
	}
	if got.RoutingBlockedStatus != nil {
		t.Errorf("routing_blocked_status not cleared: %v", got.RoutingBlockedStatus)
	}

	// And immediately claimable.
	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil || claimed == nil {
		t.Fatalf("expected claimable run after clear; err=%v claimed=%v", err, claimed)
	}
}
