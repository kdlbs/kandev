package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestResumeAdmissionActivationRequiresCurrentLiveExecution(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		state          models.TaskSessionState
		liveExecution  string
		runtimeProfile string
		wantActive     bool
	}{
		{name: "current ready execution", state: models.TaskSessionStateWaitingForInput, liveExecution: "claimed-execution", wantActive: true},
		{name: "projected predecessor candidate", state: models.TaskSessionStateWaitingForInput, liveExecution: "claimed-execution", runtimeProfile: "predecessor-profile"},
		{name: "predecessor execution", state: models.TaskSessionStateWaitingForInput, liveExecution: "predecessor-execution"},
		{name: "unknown execution", state: models.TaskSessionStateWaitingForInput},
		{name: "cancelled session", state: models.TaskSessionStateCancelled, liveExecution: "claimed-execution"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", testCase.state)
			runtimeProfile := testCase.runtimeProfile
			if runtimeProfile == "" {
				runtimeProfile = "candidate-1"
			}
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "session1", SessionID: "session1", TaskID: "task1",
				AgentExecutionID: "claimed-execution", ExecutionProfileID: runtimeProfile, Status: "ready",
			}); err != nil {
				t.Fatal(err)
			}
			engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
			decision := seedClaimedDynamicRoute(t, ctx, repo, engine, "session1", "claimed-execution")
			manager := &mockAgentManager{
				isAgentRunning: true,
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
					return testCase.liveExecution, nil
				},
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
			candidate, err := svc.loadResumeSessionCandidate(ctx, "task1", "session1", executor.ResumeOptions{}, nil)
			if testCase.wantActive {
				if err != nil || candidate == nil {
					t.Fatalf("current execution admission = %#v, %v", candidate, err)
				}
			} else {
				var blocked *sessionOpenRecoveryBlockedError
				if !errors.As(err, &blocked) || blocked.reason != autoResumeBlockedDynamicRoute || candidate != nil {
					t.Fatalf("unowned activation admission = %#v, %v; want dynamic-route block", candidate, err)
				}
			}
			state, loadErr := repo.LoadRouteState(ctx, "session1")
			if loadErr != nil || state == nil || state.Generation != decision.Generation {
				t.Fatalf("route readback = %#v, %v", state, loadErr)
			}
			if (state.Status == dynamicRouteStatusActive) != testCase.wantActive {
				t.Fatalf("route state = %s; want active %v", state.Status, testCase.wantActive)
			}
			if len(manager.stopAgentArgs) != 0 || len(manager.stopAgentWithReasonArgs) != 0 || len(manager.startAgentProcessCalls) != 0 {
				t.Fatal("activation reconciliation stopped or replaced the live execution")
			}
		})
	}
}
