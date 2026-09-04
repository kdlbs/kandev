package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	"github.com/kandev/kandev/internal/agent/settings/store"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// fakeSettingsRepo is a minimal store.Repository stand-in for building a
// ProfileExecutionResolver in these tests. None of the flag-gated code paths
// under test reach the embedded interface's methods.
type fakeSettingsRepo struct {
	store.Repository
}

func policyStateJSON(t *testing.T, deadline time.Time) string {
	t.Helper()
	raw, err := json.Marshal(dynamicruntime.PolicyState{Deadline: &deadline})
	if err != nil {
		t.Fatalf("marshal policy state: %v", err)
	}
	return string(raw)
}

func seedPendingRouteState(t *testing.T, repo *sqliterepo.Repository, sessionID string, deadline time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID:          sessionID,
		LogicalProfileID:   "dynamic-policy",
		ExecutionProfileID: "first",
		Generation:         1,
		ProfileVersion:     1,
		Status:             string(routingpolicy.DecisionRetry),
		PolicyStateJSON:    policyStateJSON(t, deadline),
		UpdatedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed pending route state: %v", err)
	}
}

func TestStartDynamicPolicyRecovery_DisabledFlagArmsNoTimer(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-flag-off", "session-flag-off", "")
	seedPendingRouteState(t, repo, "session-flag-off", time.Now().Add(time.Hour))

	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	resolver := agentruntime.NewProfileExecutionResolver(&fakeSettingsRepo{}, engine, false)
	svc := &Service{logger: testLogger(), repo: repo, profileExecutionResolver: resolver}

	svc.startDynamicPolicyRecovery(ctx)
	t.Cleanup(svc.stopDynamicPolicyRecovery)

	svc.dynamicRecoveryMu.Lock()
	timers := len(svc.dynamicRecoveryTimers)
	svc.dynamicRecoveryMu.Unlock()
	if timers != 0 {
		t.Fatalf("dynamicRecoveryTimers = %d entries, want 0 when flag disabled", timers)
	}
}

func TestStartDynamicPolicyRecovery_EnabledFlagArmsTimer(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-flag-on", "session-flag-on", "")
	seedPendingRouteState(t, repo, "session-flag-on", time.Now().Add(time.Hour))

	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	resolver := agentruntime.NewProfileExecutionResolver(&fakeSettingsRepo{}, engine, true)
	svc := &Service{logger: testLogger(), repo: repo, profileExecutionResolver: resolver}

	svc.startDynamicPolicyRecovery(ctx)
	t.Cleanup(svc.stopDynamicPolicyRecovery)

	svc.dynamicRecoveryMu.Lock()
	_, armed := svc.dynamicRecoveryTimers["session-flag-on"]
	svc.dynamicRecoveryMu.Unlock()
	if !armed {
		t.Fatal("dynamicRecoveryTimers missing session-flag-on entry when flag enabled")
	}
}

// recordingRouteStateLoader wraps the real repository to count calls to
// LoadRouteState made through the orchestrator's own dynamicRouteStateLoader
// seam. That call happens before the resolver's ResumePendingRoute is ever
// invoked, so it observes the orchestrator's own gate independently of
// ResumePendingRoute's separate, later refusal.
type recordingRouteStateLoader struct {
	*sqliterepo.Repository
	loadRouteStateCalls int
}

func (r *recordingRouteStateLoader) LoadRouteState(ctx context.Context, sessionID string) (*dynamicruntime.RouteState, error) {
	r.loadRouteStateCalls++
	return r.Repository.LoadRouteState(ctx, sessionID)
}

func TestRunDynamicPolicyRecovery_FlagFlippedOffAfterArmSkipsLaunch(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "task-flip-off", "session-flip-off", "")
	// Deadline already elapsed: firing the timer must reach the resume call,
	// not just reschedule for a later due time.
	seedPendingRouteState(t, baseRepo, "session-flip-off", time.Now().Add(-time.Minute))

	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(baseRepo), dynamicruntime.WithStateLoader(baseRepo))
	resolver := agentruntime.NewProfileExecutionResolver(&fakeSettingsRepo{}, engine, true)
	repo := &recordingRouteStateLoader{Repository: baseRepo}
	svc := &Service{
		logger: testLogger(), repo: repo, profileExecutionResolver: resolver,
		dynamicRecoveryTimers: make(map[string]*time.Timer),
	}
	// Stands in for a recovery context installed by an earlier
	// startDynamicPolicyRecovery call whose timer is now firing.
	recoveryCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.dynamicRecoveryCtx = recoveryCtx

	resolver.SetEnabled(false)
	svc.runDynamicPolicyRecovery(recoveryCtx, "session-flip-off", 1)

	if repo.loadRouteStateCalls != 0 {
		t.Fatalf("LoadRouteState calls = %d, want 0: the disabled-flag gate must return before "+
			"reading route state, not rely on ResumePendingRoute's own refusal", repo.loadRouteStateCalls)
	}

	state, err := baseRepo.LoadRouteState(ctx, "session-flip-off")
	if err != nil {
		t.Fatalf("LoadRouteState after flipped-off recovery run: %v", err)
	}
	if state == nil || state.Status != string(routingpolicy.DecisionRetry) {
		t.Fatalf("route state after flipped-off recovery run = %+v, want unchanged retry_wait", state)
	}
}

func TestLaunchDynamicRouteAction_DisabledFlagRefuses(t *testing.T) {
	ctx := context.Background()
	engine := dynamicruntime.NewEngine()
	resolver := agentruntime.NewProfileExecutionResolver(&fakeSettingsRepo{}, engine, false)
	svc := &Service{logger: testLogger(), profileExecutionResolver: resolver}

	err := svc.LaunchDynamicRouteAction(ctx, "session-launch-disabled")
	if !errors.Is(err, agentruntime.ErrDynamicRoutingDisabled) {
		t.Fatalf("LaunchDynamicRouteAction with flag off = %v, want ErrDynamicRoutingDisabled", err)
	}
}
