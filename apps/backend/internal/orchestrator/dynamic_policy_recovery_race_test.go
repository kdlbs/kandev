package orchestrator

import (
	"context"
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
// schedule or a backend-restart reschedule overlapping a still-live timer).
// Before this fix, runDynamicPolicyRecovery held no per-session guard and
// Engine.resumePending's durable claim predicate did not include the observed
// status, so both callers could claim the same retry_wait -> retrying
// transition and both launch a successor, delivering the same prompt twice.
// With the exact-status CAS (ClaimRouteStatus) plus the shared
// acquireCancelInFlightGuard now held across resolve+launch, exactly one
// caller claims the transition and only it reaches the downstream launch.
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
		dynamicruntime.WithPersistence(repo),
		dynamicruntime.WithStateLoader(repo),
	)
	return &dynamicPolicyRaceResolver{
		ProfileExecutionResolver: agentruntime.NewProfileExecutionResolver(profileRepo, engine, true),
		engine:                   engine,
	}
}
