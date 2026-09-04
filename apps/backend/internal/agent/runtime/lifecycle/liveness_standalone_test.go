package lifecycle

import (
	"context"
	"os"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// newLivenessTestManager builds a Manager whose registered standalone
// backend talks to a real fake control server, so enumeration-based liveness
// classification exercises the actual HTTP round trip.
func newLivenessTestManager(t *testing.T, control *standaloneControlServer) *Manager {
	t.Helper()
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(control.executor(t))
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	return mgr
}

func TestStandaloneExecutorNewLivenessScopeReachablePopulatesSessions(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", SessionID: "session-1"},
		{ID: "instance-2", SessionID: "session-2"},
	}
	exec := control.executor(t)

	scope := exec.newLivenessScope(context.Background())
	if !scope.reachable {
		t.Fatal("expected a successful enumeration to report reachable")
	}
	if _, ok := scope.liveBySession["session-1"]; !ok {
		t.Fatal("expected session-1 to be present in the enumeration")
	}
	if _, ok := scope.liveBySession["session-2"]; !ok {
		t.Fatal("expected session-2 to be present in the enumeration")
	}
	if len(scope.liveBySession) != 2 {
		t.Fatalf("liveBySession = %+v, want exactly 2 entries", scope.liveBySession)
	}
}

func TestStandaloneExecutorNewLivenessScopeUnreachableWhenEnumerationFails(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstancesErr = true
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(10*time.Millisecond, 0)

	scope := exec.newLivenessScope(context.Background())
	if scope.reachable {
		t.Fatal("expected an enumeration failure to report unreachable, not reachable-with-nothing")
	}
}

func TestManagerClassifyStandaloneLivenessNonStandaloneRuntimeUnchanged(t *testing.T) {
	mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
	row := &models.ExecutorRunning{SessionID: "session-1", Runtime: agentruntime.RuntimeSSH}
	scope := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{"session-1": {}}}

	got := mgr.classifyStandaloneLiveness(row, scope)
	if got != models.ProcessLivenessUnknown {
		t.Fatalf("classifyStandaloneLiveness for a non-standalone row = %v, want Unknown regardless of scope", got)
	}
}

func TestManagerClassifyStandaloneLivenessNothingAnsweredFallsBackToProcessProbe(t *testing.T) {
	mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
	aliveRow := &models.ExecutorRunning{
		SessionID: "session-alive", Runtime: agentruntime.RuntimeStandalone, LocalPID: os.Getpid(),
	}
	deadRow := &models.ExecutorRunning{
		SessionID: "session-dead", Runtime: agentruntime.RuntimeStandalone, LocalPID: spawnAndReapPID(t),
	}

	for _, scope := range []*standaloneLivenessScope{nil, {reachable: false}} {
		if got := mgr.classifyStandaloneLiveness(aliveRow, scope); got != models.ProcessLivenessAlive {
			t.Fatalf("scope=%+v: classifyStandaloneLiveness(aliveRow) = %v, want Alive via process-identifier fallback", scope, got)
		}
		if got := mgr.classifyStandaloneLiveness(deadRow, scope); got != models.ProcessLivenessDead {
			t.Fatalf("scope=%+v: classifyStandaloneLiveness(deadRow) = %v, want Dead via process-identifier fallback", scope, got)
		}
	}
}

func TestManagerClassifyStandaloneLivenessPresentNoStopInFlightIsAlive(t *testing.T) {
	mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
	row := &models.ExecutorRunning{SessionID: "session-present", Runtime: agentruntime.RuntimeStandalone}
	scope := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{"session-present": {}}}

	got := mgr.classifyStandaloneLiveness(row, scope)
	if got != models.ProcessLivenessAlive {
		t.Fatalf("classifyStandaloneLiveness = %v, want Alive for a present instance with no stop in flight", got)
	}
}

func TestManagerClassifyStandaloneLivenessPresentWithStopInFlightIsUnknown(t *testing.T) {
	mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
	row := &models.ExecutorRunning{SessionID: "session-stopping", Runtime: agentruntime.RuntimeStandalone}
	scope := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{"session-stopping": {}}}
	mgr.recoveryGuard.AcquireOrObserve("session-stopping")
	mgr.recoveryGuard.MarkStopInFlight("session-stopping")

	got := mgr.classifyStandaloneLiveness(row, scope)
	if got != models.ProcessLivenessUnknown {
		t.Fatalf("classifyStandaloneLiveness = %v, want Unknown when the row's stop is still in flight", got)
	}
}

func TestManagerClassifyStandaloneLivenessAbsentOwnRecordIsDead(t *testing.T) {
	mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
	row := &models.ExecutorRunning{SessionID: "session-own", Runtime: agentruntime.RuntimeStandalone}
	scope := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{}}
	mgr.markSessionCreatedThisLifetime("session-own")

	got := mgr.classifyStandaloneLiveness(row, scope)
	if got != models.ProcessLivenessDead {
		t.Fatalf("classifyStandaloneLiveness = %v, want Dead for an own record absent from the enumeration", got)
	}
}

func TestManagerClassifyStandaloneLivenessAbsentInheritedRecordIsUnknown(t *testing.T) {
	mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
	row := &models.ExecutorRunning{SessionID: "session-inherited", Runtime: agentruntime.RuntimeStandalone}
	scope := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{}}
	// Deliberately NOT marked via markSessionCreatedThisLifetime: this is an
	// inherited row this backend never created.

	got := mgr.classifyStandaloneLiveness(row, scope)
	if got != models.ProcessLivenessUnknown {
		t.Fatalf("classifyStandaloneLiveness = %v, want Unknown for an inherited record absent from the enumeration", got)
	}
}

func TestManagerRowLivenessTakesFreshEnumerationEachCall(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{{ID: "instance-1", SessionID: "session-1"}}
	mgr := newLivenessTestManager(t, control)
	// A caller outside Start's recovery pass only enumerates once recovery
	// has completed for this process's lifetime (AC-EXECUTORS-SURVIVAL-003.6).
	mgr.recoveryComplete.Store(true)
	row := &models.ExecutorRunning{SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone}

	mgr.RowLiveness(row)
	mgr.RowLiveness(row)

	control.mu.Lock()
	defer control.mu.Unlock()
	if control.listInstancesAttempts != 2 {
		t.Fatalf("listInstances attempts = %d, want exactly 2 (one fresh enumeration per out-of-pass call, never cached)", control.listInstancesAttempts)
	}
}

// TestManagerRowLivenessReturnsUnknownBeforeRecoveryCompletes pins Review
// round 1 finding 8 (AC-EXECUTORS-SURVIVAL-003.6): a caller outside a
// reconciliation pass (e.g. idle reclaim) firing before Start's recovery
// pass has finished for this process's lifetime must get Unknown immediately
// -- neither enumerate nor wait -- rather than racing a live enumeration
// against work recovery itself has not finished doing.
func TestManagerRowLivenessReturnsUnknownBeforeRecoveryCompletes(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{{ID: "instance-1", SessionID: "session-1"}}
	mgr := newLivenessTestManager(t, control)
	row := &models.ExecutorRunning{SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone}

	got := mgr.RowLiveness(row)
	if got != models.ProcessLivenessUnknown {
		t.Fatalf("RowLiveness before recovery completes = %v, want Unknown", got)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if control.listInstancesAttempts != 0 {
		t.Fatalf("listInstances attempts = %d, want 0 (must not enumerate before recovery completes)", control.listInstancesAttempts)
	}
}

func TestManagerRowLivenessScopedReusesSuppliedScope(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{{ID: "instance-1", SessionID: "session-1"}}
	mgr := newLivenessTestManager(t, control)
	rowA := &models.ExecutorRunning{SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone}
	rowB := &models.ExecutorRunning{SessionID: "session-2", Runtime: agentruntime.RuntimeStandalone}

	scope := mgr.NewStandaloneLivenessScope(context.Background())
	mgr.RowLivenessScoped(rowA, scope)
	mgr.RowLivenessScoped(rowB, scope)

	control.mu.Lock()
	defer control.mu.Unlock()
	if control.listInstancesAttempts != 1 {
		t.Fatalf("listInstances attempts = %d, want exactly 1 -- one enumeration reused across every row of the pass", control.listInstancesAttempts)
	}
}
