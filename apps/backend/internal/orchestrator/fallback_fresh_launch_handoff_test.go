package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// TestFallbackFreshLaunch_ComposedPromptSurvivesStepRecomposition covers the
// ErrExecutionNotFound recovery path from autoStartStepPrompt: when the
// caller has already composed the dispatch prompt (appending a claimed step
// handoff), the fresh launch must not let StartCreatedSession's own workflow
// step recomposition discard it. A destination step Prompt template with no
// {{task_prompt}} placeholder replaces any base prompt during recomposition
// (see TestApplyWorkflowAndPlanMode_KeepsWorkflowPromptVisibleWhenStepEnablesPlanMode
// in workflow_prompt_test.go), so promptAlreadyComposed must route around
// that seam entirely instead of losing the handoff.
func TestFallbackFreshLaunch_ComposedPromptSurvivesStepRecomposition(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-fallback-handoff"
		sessionID = "session-fallback-handoff"
		stepID    = "step-next"
		handoff   = "watch out for the flaky test"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	dbTask, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	dbTask.WorkflowStepID = stepID
	require.NoError(t, repo.UpdateTask(ctx, dbTask))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-fallback"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Name: "Next",
		Prompt: "Recomposed step instructions.",
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, Title: "Test Task", State: v1.TaskStateInProgress}

	var launchedDescription string
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchedDescription = req.TaskDescription
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-fresh"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	svc.messageCreator = &mockMessageCreator{}

	composedPrompt := "Run the next workflow step.\n\n" + stepHandoffPromptHeading + "\n\n" + handoff

	err = svc.fallbackFreshLaunchOnMissingExecution(
		ctx, taskID, sessionID, composedPrompt, true, composedPrompt, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.Contains(t, launchedDescription, handoff,
		"the already-composed prompt (with the claimed handoff) must survive the fresh launch")
	require.NotEqual(t, "Recomposed step instructions.", strings.TrimSpace(launchedDescription),
		"promptAlreadyComposed must skip StartCreatedSession's own step-template recomposition")
}
