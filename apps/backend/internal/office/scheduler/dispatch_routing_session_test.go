package scheduler_test

import (
	"context"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
)

// fakeTaskStarterWithSession wraps fakeTaskStarter and additionally
// implements scheduler.TaskStarterWithSession, so dispatch tests can
// drive the session-returning seam (AC-OFFICE-LOOP-LIVENESS-002.7)
// without touching the production orchestrator adapter.
type fakeTaskStarterWithSession struct {
	*fakeTaskStarter
	sessionID    string
	sessionErr   error
	calledRoute  scheduler.RouteOverride
	returnedCall bool
}

func (f *fakeTaskStarterWithSession) StartTaskWithRouteReturningSession(
	ctx context.Context, taskID, agentID string,
	launch scheduler.LaunchContext, route scheduler.RouteOverride,
) (string, error) {
	f.returnedCall = true
	f.calledRoute = route
	if err := f.StartTaskWithRoute(ctx, taskID, agentID, launch, route); err != nil {
		return "", err
	}
	if f.sessionErr != nil {
		return "", f.sessionErr
	}
	return f.sessionID, nil
}

// expvarMapInt reads one label key out of a package-global expvar.Map,
// returning 0 when the key was never incremented. Used to assert
// counter deltas without a reset seam — expvar's registry is global and
// process-wide, so this reads exactly what production code would.
func expvarMapInt(t *testing.T, mapName, key string) int64 {
	t.Helper()
	v := expvar.Get(mapName)
	if v == nil {
		t.Fatalf("expvar map %q not registered", mapName)
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("expvar %q is not a Map", mapName)
	}
	sub := m.Get(key)
	if sub == nil {
		return 0
	}
	iv, ok := sub.(*expvar.Int)
	if !ok {
		t.Fatalf("expvar %q[%q] is not an Int", mapName, key)
	}
	return iv.Value()
}

// AC-OFFICE-LOOP-LIVENESS-002.7: on a routed launch, the session id the
// starter returns is persisted onto the run row.
func TestDispatch_RoutedLaunchPersistsSessionID(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := &fakeTaskStarterWithSession{fakeTaskStarter: newFakeTaskStarter(), sessionID: "sess-routed-1"}
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-session-1"}`)

	before := expvarMapInt(t, "office_loop_launch_total", "workspace="+testWorkspaceID)

	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil || !launched || parked {
		t.Fatalf("launched=%v parked=%v err=%v, want launched", launched, parked, err)
	}
	if !starter.returnedCall {
		t.Fatal("expected StartTaskWithRouteReturningSession to be called")
	}

	updated, err := repo.GetRunByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.SessionID != "sess-routed-1" {
		t.Fatalf("session_id = %q, want sess-routed-1", updated.SessionID)
	}

	after := expvarMapInt(t, "office_loop_launch_total", "workspace="+testWorkspaceID)
	if after != before+1 {
		t.Fatalf("office_loop_launch_total delta = %d, want 1", after-before)
	}
}

// AC-002.8: an empty returned session id is left empty and counted
// under launch_without_session, never replaced with a placeholder.
func TestDispatch_RoutedLaunchEmptySessionCountsWithoutSession(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := &fakeTaskStarterWithSession{fakeTaskStarter: newFakeTaskStarter(), sessionID: ""}
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-session-2"}`)

	before := expvarMapInt(t, "office_loop_launch_without_session_total", "workspace="+testWorkspaceID)

	launched, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil || !launched {
		t.Fatalf("launched=%v err=%v, want launched", launched, err)
	}

	updated, err := repo.GetRunByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.SessionID != "" {
		t.Fatalf("session_id = %q, want empty (no placeholder)", updated.SessionID)
	}
	after := expvarMapInt(t, "office_loop_launch_without_session_total", "workspace="+testWorkspaceID)
	if after != before+1 {
		t.Fatalf("office_loop_launch_without_session_total delta = %d, want 1", after-before)
	}
}

// AC-002.10: relaunching the same run with a new session id overwrites
// the previous one — last non-empty wins within one claim.
func TestDispatch_RoutedRelaunchKeepsLatestSessionID(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := &fakeTaskStarterWithSession{fakeTaskStarter: newFakeTaskStarter(), sessionID: "sess-first"}
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-session-3"}`)

	if _, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{}); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	starter.sessionID = "sess-second"
	// Re-fetch: DispatchWithRouting mutates run.LogicalProviderOrder in
	// place, so reuse the same struct to simulate a relaunch of the
	// same claimed row.
	if _, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{}); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}

	updated, err := repo.GetRunByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.SessionID != "sess-second" {
		t.Fatalf("session_id = %q, want sess-second (last non-empty wins)", updated.SessionID)
	}
}
