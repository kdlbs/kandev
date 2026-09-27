package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPromptTask_RejectsModelOutsideExactProfileAssignment(t *testing.T) {
	for _, supportsInPlaceSwitch := range []bool{true, false} {
		name := "replacement launch"
		if supportsInPlaceSwitch {
			name = "in-place selection"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-exact-model", "session-exact-model", "step1")
			profileRevision := time.Now().UTC().Round(0)
			if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
				TaskID: "task-exact-model", WorkspaceID: "ws1", AgentProfileID: "profile-exact",
				ProfileRevision: profileRevision, Generation: 1,
			}); err != nil {
				t.Fatalf("assign exact profile: %v", err)
			}

			session, err := repo.GetTaskSession(ctx, "session-exact-model")
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			session.State = models.TaskSessionStateWaitingForInput
			session.AgentProfileID = "profile-exact"
			session.ExactProfileGeneration = 1
			session.ExactProfileRevision = profileRevision.UnixNano()
			session.AgentProfileSnapshot = map[string]interface{}{"model": "exact-model"}
			if err := repo.UpdateTaskSession(ctx, session); err != nil {
				t.Fatalf("update session: %v", err)
			}
			seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-exact-model")

			var launchCalls int
			agentMgr := &mockAgentManager{
				isAgentRunning:           true,
				setSessionModelSupported: supportsInPlaceSwitch,
				resolveProfileInfo: &executor.AgentProfileInfo{
					ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: true,
					Revision: profileRevision, Model: "exact-model",
				},
				launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					launchCalls++
					return &executor.LaunchAgentResponse{AgentExecutionID: "replacement-execution"}, nil
				},
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

			_, err = svc.PromptTask(ctx, session.TaskID, session.ID, "continue", "client-selected-model", false, nil, false)
			if !errors.Is(err, ErrExactProfileModelMismatch) {
				t.Fatalf("PromptTask error = %v, want exact profile assignment rejection", err)
			}
			if len(agentMgr.setSessionModelCalls) != 0 {
				t.Fatalf("model selection calls = %#v, want none", agentMgr.setSessionModelCalls)
			}
			if len(agentMgr.startAgentProcessCalls) != 0 || launchCalls != 0 {
				t.Fatalf("replacement startup = (%#v, %d), want none", agentMgr.startAgentProcessCalls, launchCalls)
			}
			if len(agentMgr.capturedPrompts) != 0 {
				t.Fatalf("prompt calls = %#v, want none", agentMgr.capturedPrompts)
			}
		})
	}
}
