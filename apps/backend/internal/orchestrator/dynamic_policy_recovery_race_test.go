package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestConcurrentDynamicPolicyRecoveryLaunchesExactlyOnce reproduces two
// recovery-timer fires racing on the same due generation (e.g. a duplicate
// schedule or a backend-restart reschedule overlapping a still-live timer)
// within one Service. Both goroutines share svc.acquireCancelInFlightGuard,
// so they are strictly serialized before either reaches the durable claim:
// the loser's fresh loadDueDynamicPolicyState re-read then sees the winner's
// "retrying" status and returns early, never reaching ResumePendingRoute at
// all. The exact-status CAS in ClaimRouteStateFrom is never exercised here —
// see TestConcurrentDynamicPolicyRecoveryClaimContendsOnSQLCAS for a test
// that forces two independent Service instances to actually contend on it.
func TestConcurrentDynamicPolicyRecoveryLaunchesExactlyOnce(t *testing.T) {
	ctx := context.Background()
	const (
		taskID            = "task-policy-race"
		sessionID         = "session-policy-race"
		executionID       = "execution-policy-race"
		dynamicProfileID  = "profile-dynamic-race"
		concreteProfileID = "profile-concrete-race"
	)

	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step-race")

	resolver := newDynamicPolicyRaceResolver(t, repo, dynamicProfileID, concreteProfileID)

	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = dynamicProfileID
	session.ExecutionProfileID = concreteProfileID
	session.RouteGeneration = 4
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	elapsedDeadline := time.Now().UTC().Add(-time.Minute)
	if err := repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID: sessionID, LogicalProfileID: dynamicProfileID,
		ExecutionProfileID: concreteProfileID, Generation: 4, ProfileVersion: 1,
		Status:          "retry_wait",
		PolicyStateJSON: `{"deadline":"` + elapsedDeadline.Format(time.RFC3339Nano) + `"}`,
		UpdatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveRouteState: %v", err)
	}
	if _, _, err := resolver.EngineForTest().LoadState(ctx, sessionID); err != nil {
		t.Fatalf("warm engine cache: %v", err)
	}

	var launches int32
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			atomic.AddInt32(&launches, 1)
			return &executor.LaunchAgentResponse{AgentExecutionID: "relaunch-" + req.SessionID}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-race"] = &wfmodels.WorkflowStep{ID: "step-race", WorkflowID: "wf1", Name: "Work"}

	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
	svc.SetProfileExecutionResolver(resolver.ProfileExecutionResolver)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			svc.runDynamicPolicyRecovery(ctx, sessionID, 4)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&launches); got != 1 {
		t.Fatalf("successor launches = %d, want exactly 1 (duplicate prompt delivery)", got)
	}
	agentManager.mu.Lock()
	stopCalls := len(agentManager.stopAgentWithReasonArgs)
	agentManager.mu.Unlock()
	if stopCalls != 1 {
		t.Fatalf("predecessor stop calls = %d, want exactly 1", stopCalls)
	}
}

// dynamicPolicyRaceResolver exposes the underlying engine so the test can
// warm its in-memory cache exactly as production restart-recovery does via
// LoadState, before firing the two racing claims.
type dynamicPolicyRaceResolver struct {
	*agentruntime.ProfileExecutionResolver
	engine *dynamicruntime.Engine
}

func (r *dynamicPolicyRaceResolver) EngineForTest() *dynamicruntime.Engine { return r.engine }

func newDynamicPolicyRaceResolver(
	t *testing.T, repo *sqliterepo.Repository, dynamicProfileID, concreteProfileID string,
) *dynamicPolicyRaceResolver {
	return newDynamicPolicyRaceResolverWithPersistence(t, repo, dynamicProfileID, concreteProfileID, repo)
}

// newDynamicPolicyRaceResolverWithPersistence lets a caller substitute the
// engine's durable persistence (e.g. a rendezvous barrier around
// ClaimRouteStateFrom) while state reads still go through the real repo.
func newDynamicPolicyRaceResolverWithPersistence(
	t *testing.T, repo *sqliterepo.Repository, dynamicProfileID, concreteProfileID string,
	persistence dynamicruntime.Persistence,
) *dynamicPolicyRaceResolver {
	t.Helper()
	dbPath := t.TempDir() + "/agent-settings.db"
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	profileRepo, cleanup, err := agentsettingsstore.Provide(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	ctx := context.Background()
	for _, agent := range []*agentsettingsmodels.Agent{
		{ID: "dynamic", Name: "dynamic"},
		{ID: "concrete-agent", Name: "concrete-agent"},
	} {
		if err := profileRepo.CreateAgent(ctx, agent); err != nil {
			t.Fatal(err)
		}
	}
	for _, profile := range []*agentsettingsmodels.AgentProfile{
		{ID: dynamicProfileID, AgentID: "dynamic", Name: "Cascade", Enabled: true},
		{ID: concreteProfileID, AgentID: "concrete-agent", Name: "Concrete", Enabled: true},
	} {
		if err := profileRepo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	if err := profileRepo.CreateDynamicAgentProfile(ctx,
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: dynamicProfileID, Version: 1},
		[]agentsettingsmodels.DynamicAgentRoute{{
			DynamicProfileID:   dynamicProfileID,
			ExecutionProfileID: concreteProfileID,
			Enabled:            true,
		}},
	); err != nil {
		t.Fatal(err)
	}
	engine := dynamicruntime.NewEngine(
		dynamicruntime.WithPersistence(persistence),
		dynamicruntime.WithStateLoader(repo),
	)
	return &dynamicPolicyRaceResolver{
		ProfileExecutionResolver: agentruntime.NewProfileExecutionResolver(profileRepo, engine, true),
		engine:                   engine,
	}
}

// claimRendezvousBarrier wraps the real repository's durable claim so the
// first two concurrent callers are guaranteed to both arrive at
// ClaimRouteStateFrom before either one executes it, rather than leaving
// that to goroutine scheduling luck. Only that first pair rendezvous: a
// buggy claim can let both callers proceed to a downstream recovery path
// (e.g. markDynamicRouteActionRequired) that reaches this same method again,
// and those later calls pass straight through rather than waiting for a
// third party that will never arrive. It forwards every other
// Persistence/StateLoader method straight to the embedded repository.
type claimRendezvousBarrier struct {
	*sqliterepo.Repository
	t *testing.T

	mu      sync.Mutex
	seen    int
	release chan struct{}
}

func newClaimRendezvousBarrier(t *testing.T, repo *sqliterepo.Repository) *claimRendezvousBarrier {
	return &claimRendezvousBarrier{Repository: repo, t: t}
}

// rendezvous blocks the first arrival until a second one arrives, then
// releases both together; the third and any later arrival never blocks. It
// reports (via t.Errorf, safe from any goroutine) and returns false if no
// second caller shows up within the timeout, so a regression that stops a
// caller short of the claim fails the test instead of hanging it.
func (b *claimRendezvousBarrier) rendezvous() bool {
	b.mu.Lock()
	b.seen++
	seen := b.seen
	if seen == 1 {
		b.release = make(chan struct{})
	}
	release := b.release
	if seen == 2 {
		close(release)
	}
	b.mu.Unlock()
	if seen != 1 {
		return true
	}
	select {
	case <-release:
		return true
	case <-time.After(5 * time.Second):
		b.t.Errorf("claim rendezvous timed out waiting for a second caller")
		return false
	}
}

func (b *claimRendezvousBarrier) ClaimRouteStateFrom(
	ctx context.Context,
	expectedGeneration int64,
	expectedStatus string,
	state dynamicruntime.RouteState,
) (bool, error) {
	if !b.rendezvous() {
		return false, errors.New("claim rendezvous timed out waiting for a second caller")
	}
	return b.Repository.ClaimRouteStateFrom(ctx, expectedGeneration, expectedStatus, state)
}

// TestConcurrentDynamicPolicyRecoveryClaimContendsOnSQLCAS complements
// TestConcurrentDynamicPolicyRecoveryLaunchesExactlyOnce: that test's two
// goroutines share one Service, so svc.acquireCancelInFlightGuard serializes
// them before either reaches the durable claim and the SQL CAS in
// ClaimRouteStateFrom is never the discriminator. This test uses two
// independent Service instances -- as two separate backend processes racing
// the same durable row would -- each with its own cancelInFlight map and its
// own Engine (cache-warmed independently), sharing only the underlying
// repository. A rendezvous barrier around ClaimRouteStateFrom forces both to
// arrive at the durable claim before either executes it, so only the SQL
// predicate decides the winner.
func TestConcurrentDynamicPolicyRecoveryClaimContendsOnSQLCAS(t *testing.T) {
	ctx := context.Background()
	const (
		taskID            = "task-policy-race-cas"
		sessionID         = "session-policy-race-cas"
		executionID       = "execution-policy-race-cas"
		dynamicProfileID  = "profile-dynamic-race-cas"
		concreteProfileID = "profile-concrete-race-cas"
	)

	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step-race")

	barrier := newClaimRendezvousBarrier(t, repo)
	resolverA := newDynamicPolicyRaceResolverWithPersistence(t, repo, dynamicProfileID, concreteProfileID, barrier)
	resolverB := newDynamicPolicyRaceResolverWithPersistence(t, repo, dynamicProfileID, concreteProfileID, barrier)

	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = dynamicProfileID
	session.ExecutionProfileID = concreteProfileID
	session.RouteGeneration = 4
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	elapsedDeadline := time.Now().UTC().Add(-time.Minute)
	if err := repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID: sessionID, LogicalProfileID: dynamicProfileID,
		ExecutionProfileID: concreteProfileID, Generation: 4, ProfileVersion: 1,
		Status:          "retry_wait",
		PolicyStateJSON: `{"deadline":"` + elapsedDeadline.Format(time.RFC3339Nano) + `"}`,
		UpdatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveRouteState: %v", err)
	}
	if _, _, err := resolverA.EngineForTest().LoadState(ctx, sessionID); err != nil {
		t.Fatalf("warm engine A cache: %v", err)
	}
	if _, _, err := resolverB.EngineForTest().LoadState(ctx, sessionID); err != nil {
		t.Fatalf("warm engine B cache: %v", err)
	}

	var launches int32
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			atomic.AddInt32(&launches, 1)
			return &executor.LaunchAgentResponse{AgentExecutionID: "relaunch-" + req.SessionID}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-race"] = &wfmodels.WorkflowStep{ID: "step-race", WorkflowID: "wf1", Name: "Work"}

	svcA := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
	svcA.SetProfileExecutionResolver(resolverA.ProfileExecutionResolver)
	svcA.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	svcB := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
	svcB.SetProfileExecutionResolver(resolverB.ProfileExecutionResolver)
	svcB.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		svcA.runDynamicPolicyRecovery(ctx, sessionID, 4)
	}()
	go func() {
		defer wg.Done()
		svcB.runDynamicPolicyRecovery(ctx, sessionID, 4)
	}()
	wg.Wait()

	if got := atomic.LoadInt32(&launches); got != 1 {
		t.Fatalf("successor launches = %d, want exactly 1 (duplicate prompt delivery)", got)
	}
	agentManager.mu.Lock()
	stopCalls := len(agentManager.stopAgentWithReasonArgs)
	agentManager.mu.Unlock()
	if stopCalls != 1 {
		t.Fatalf("predecessor stop calls = %d, want exactly 1", stopCalls)
	}
}
