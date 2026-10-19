package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type failingRouteProjectionRepo struct {
	*sqliterepo.Repository
	failProjection bool
}

func (r *failingRouteProjectionRepo) UpdateTaskSessionDynamicRouteIfCurrent(
	ctx context.Context, id string, generation int64, expectedState, state, reason string,
) (bool, time.Time, error) {
	if r.failProjection {
		return false, time.Time{}, errors.New("injected session route projection failure")
	}
	return r.Repository.UpdateTaskSessionDynamicRouteIfCurrent(ctx, id, generation, expectedState, state, reason)
}

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.12
func TestResumeAdmissionWaitsForPersistedActiveRouteProjection(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session1", SessionID: "session1", TaskID: "task1",
		AgentExecutionID: "claimed-execution", ExecutionProfileID: "candidate-1", Status: "ready",
	}); err != nil {
		t.Fatal(err)
	}
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, "session1", "claimed-execution")
	manager := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	projection := &failingRouteProjectionRepo{Repository: repo, failProjection: true}
	svc.repo = projection

	candidate, err := svc.loadResumeSessionCandidate(ctx, "task1", "session1", executor.ResumeOptions{}, nil)
	var blocked *sessionOpenRecoveryBlockedError
	if candidate != nil || !errors.As(err, &blocked) || blocked.reason != autoResumeBlockedDynamicRoute {
		t.Fatalf("admission with failed projection = %#v, %v; want the pending route guard", candidate, err)
	}
	state, err := repo.LoadRouteState(ctx, "session1")
	if err != nil || state == nil || state.Status != dynamicRouteStatusActive || state.Generation != decision.Generation {
		t.Fatalf("durable activation = %#v, %v; want the same generation active", state, err)
	}
	if persisted := mustTaskSession(t, repo, ctx, "session1"); persisted.RouteState != dynamicRouteStatusStarting {
		t.Fatalf("failed projection unexpectedly changed the persisted session: %#v", persisted)
	}

	projection.failProjection = false
	candidate, err = svc.loadResumeSessionCandidate(ctx, "task1", "session1", executor.ResumeOptions{}, nil)
	if err != nil || candidate == nil || candidate.session.RouteState != dynamicRouteStatusActive {
		t.Fatalf("admission after projection recovery = %#v, %v; want active", candidate, err)
	}
	persisted := mustTaskSession(t, repo, ctx, "session1")
	if persisted.RouteGeneration != decision.Generation || persisted.RouteState != dynamicRouteStatusActive ||
		candidate.session.RouteReason != persisted.RouteReason || !candidate.session.UpdatedAt.Equal(persisted.UpdatedAt) {
		t.Fatalf("admission differs from the authoritative projection: %#v / %#v", candidate.session, persisted)
	}
	if len(manager.stopAgentArgs) != 0 || len(manager.stopAgentWithReasonArgs) != 0 || len(manager.startAgentProcessCalls) != 0 {
		t.Fatal("projection recovery stopped or replaced the live execution")
	}
}
