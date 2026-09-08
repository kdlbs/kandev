package scheduler_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
)

// newTestRepoSchedRejectingSessionWrite is newTestRepoSched plus a
// trigger that aborts only an UPDATE setting runs.session_id to
// wantSessionID — the exact value the launch under test will produce —
// so a test can force SetRunSessionID to fail (AC-002.11) without
// disturbing any other write persistLaunchedSession's callers depend on
// (route-attempt bookkeeping, resolved-route persistence).
func newTestRepoSchedRejectingSessionWrite(t *testing.T, wantSessionID string) *officesqlite.Repository {
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
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TRIGGER reject_session_id_write
		BEFORE UPDATE OF session_id ON runs
		WHEN NEW.session_id = '%s'
		BEGIN
			SELECT RAISE(ABORT, 'session_id write rejected for test');
		END;
	`, wantSessionID)); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	return repo
}

// AC-002.11: on a routed launch, a session persist failure is counted
// separately from a without-session launch, and does not fail the
// launch itself or overwrite the run's session_id — the agent is
// already running by the time this write happens.
func TestDispatch_RoutedLaunchSessionPersistFailureCountsSeparately(t *testing.T) {
	repo := newTestRepoSchedRejectingSessionWrite(t, "sess-routed-persist-fail")
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := &fakeTaskStarterWithSession{fakeTaskStarter: newFakeTaskStarter(), sessionID: "sess-routed-persist-fail"}
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-session-persist-fail"}`)

	beforeFailed := expvarMapInt(t, "office_loop_session_persist_failed_total", "workspace="+testWorkspaceID)
	beforeWithoutSession := expvarMapInt(t, "office_loop_launch_without_session_total", "workspace="+testWorkspaceID)

	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil || !launched || parked {
		t.Fatalf("launched=%v parked=%v err=%v, want launched despite the persist failure", launched, parked, err)
	}

	updated, err := repo.GetRunByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.SessionID != "" {
		t.Fatalf("session_id = %q, want unchanged/empty after a rejected write", updated.SessionID)
	}

	afterFailed := expvarMapInt(t, "office_loop_session_persist_failed_total", "workspace="+testWorkspaceID)
	if afterFailed != beforeFailed+1 {
		t.Fatalf("office_loop_session_persist_failed_total delta = %d, want 1", afterFailed-beforeFailed)
	}
	afterWithoutSession := expvarMapInt(t, "office_loop_launch_without_session_total", "workspace="+testWorkspaceID)
	if afterWithoutSession != beforeWithoutSession {
		t.Fatalf("office_loop_launch_without_session_total delta = %d, want 0", afterWithoutSession-beforeWithoutSession)
	}
}
