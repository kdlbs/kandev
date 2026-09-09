package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// TestHandlePromptDispatchFailure_ComposedPromptSurvivesInternalFallback
// covers promptTask's OWN internal ErrExecutionNotFound recovery
// (handlePromptDispatchFailure), a second, independent trigger for
// fallbackFreshLaunchOnMissingExecution beyond autoStartStepPrompt's own
// explicit branch (covered by
// TestFallbackFreshLaunch_ComposedPromptSurvivesStepRecomposition). This
// internal recovery fires on the ordinary "lazy resume after a backend
// restart" race (promptTask's resumedForPrompt) and used to always call
// fallbackFreshLaunchOnMissingExecution with promptAlreadyComposed=false,
// silently discarding an already-claimed step handoff whenever the caller
// had already composed the dispatch prompt (e.g. autoStartStepPrompt,
// appending a claimed handoff before calling promptTask) — the same failure
// mode Build round 4 fixed at the sibling call site only.
//
// Exercises handlePromptDispatchFailure directly rather than through
// promptTask/PromptTask: promptTask's requireNonterminalSession branch
// already holds the per-session cancelInFlightGuard for the rest of the
// call, and fallbackFreshLaunchOnMissingExecution re-acquires that same
// guard — a pre-existing, unrelated locking shape (present at merge-base,
// unrelated to this capability) that this test must not exercise.
func TestHandlePromptDispatchFailure_ComposedPromptSurvivesInternalFallback(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-dispatch-failure-handoff"
		sessionID = "session-dispatch-failure-handoff"
		stepID    = "step-next-dispatch-failure"
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
	session.AgentProfileID = "profile-dispatch-failure"
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

	// Mirrors the guard handlePromptDispatchFailure checks before recovering
	// internally: resumedForPrompt=true, nothing accepted or reserved yet,
	// and the specific ErrExecutionNotFound this recovery targets.
	result, err := svc.handlePromptDispatchFailure(
		ctx, taskID, sessionID, composedPrompt,
		true, true,
		nil,
		promptClaimRollback{},
		false, false,
		executor.ErrExecutionNotFound,
		true, sysprompt.InjectPlanMode(composedPrompt), composedPrompt,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, launchedDescription, handoff,
		"the already-composed prompt (with the claimed handoff) must survive promptTask's own internal fallback")
	require.Contains(t, launchedDescription, sysprompt.PlanMode(),
		"the fallback must retain the plan-mode transform")
	require.NotEqual(t, "Recomposed step instructions.", strings.TrimSpace(launchedDescription),
		"promptAlreadyComposed must skip StartCreatedSession's own step-template recomposition")
}
