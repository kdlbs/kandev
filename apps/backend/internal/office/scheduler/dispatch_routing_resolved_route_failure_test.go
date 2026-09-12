package scheduler_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
)

// newTestRepoSchedRejectingResolvedRouteWrite is newTestRepoSched plus a
// trigger that aborts every write to resolved_execution_profile_id, so a
// test can force SetRunResolvedRoute to fail without disturbing the
// session_id write persistLaunchedSession performs — the two writes
// this test needs to tell apart.
func newTestRepoSchedRejectingResolvedRouteWrite(t *testing.T) *officesqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER reject_resolved_route_write
		BEFORE UPDATE OF resolved_execution_profile_id ON runs
		BEGIN
			SELECT RAISE(ABORT, 'resolved route write rejected for test');
		END;
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	return repo
}

// AC-002.7 / AC-002.11 / AC-003.9 (Review round 3, R3-2): a launch that
// already started a real agent session must not lose that session's
// correlation, or go uncounted as a launch, just because an unrelated
// bookkeeping write (the resolved provider/model snapshot) fails
// afterward. Before the fix, handleLaunchSuccess returned early on a
// SetRunResolvedRoute error, before ever reaching persistLaunchedSession
// or IncLoopLaunch — silently abandoning a live, now-uncorrelated
// session and under-counting a launch that genuinely happened.
func TestDispatch_RoutedLaunchSurvivesResolvedRouteWriteFailure(t *testing.T) {
	repo := newTestRepoSchedRejectingResolvedRouteWrite(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := &fakeTaskStarterWithSession{fakeTaskStarter: newFakeTaskStarter(), sessionID: "sess-resolved-route-fail"}
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-resolved-route-fail"}`)

	beforeLaunch := expvarMapInt(t, "office_loop_launch_total", "workspace="+testWorkspaceID)

	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil || !launched || parked {
		t.Fatalf("launched=%v parked=%v err=%v, want launched despite the resolved-route write failure", launched, parked, err)
	}
	if !starter.returnedCall {
		t.Fatal("expected StartTaskWithRouteReturningSession to be called")
	}

	updated, err := repo.GetRunByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.SessionID != "sess-resolved-route-fail" {
		t.Fatalf("session_id = %q, want sess-resolved-route-fail (a live launch must not lose its session correlation)", updated.SessionID)
	}

	afterLaunch := expvarMapInt(t, "office_loop_launch_total", "workspace="+testWorkspaceID)
	if afterLaunch != beforeLaunch+1 {
		t.Fatalf("office_loop_launch_total delta = %d, want 1 (the launch genuinely happened)", afterLaunch-beforeLaunch)
	}
}
