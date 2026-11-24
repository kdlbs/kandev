package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	moveSavedPromptInstructions = "Follow @review-checklist and @unknown."
	moveSavedPromptDefinition   = "Review the changed tests."
)

// @covers AC-TASKS-SAVED-PROMPT-DELIVERY-001.17, AC-TASKS-SAVED-PROMPT-DELIVERY-001.18
func TestMoveInstructionSavedPromptFreshLaunch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		skipStep bool
		hasStep  bool
	}{
		{name: "composed", hasStep: true},
		{name: "skip step", skipStep: true, hasStep: true},
		{name: "direct fallback"},
		{name: "skip step without a loaded step", skipStep: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			const stepID = "step-review"
			const durablePrompt = "Review this task using @step-checklist."
			prompts := newPromptServiceForLaunchFallbackTest(t)
			_, err := prompts.CreatePrompt(ctx, "review-checklist", moveSavedPromptDefinition)
			require.NoError(t, err)
			_, err = prompts.CreatePrompt(ctx, "step-checklist", "Check the durable step policy.")
			require.NoError(t, err)

			repo := setupTestRepo(t)
			seedTaskWithoutSession(t, repo, "task1", stepID)
			dbTask, err := repo.GetTask(ctx, "task1")
			require.NoError(t, err)
			dbTask.Description = "fallback description"
			require.NoError(t, repo.UpdateTask(ctx, dbTask))
			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task1"] = &v1.Task{
				ID: "task1", Title: "Task", Description: "fallback description", State: v1.TaskStateInProgress,
			}
			stepGetter := newMockStepGetter()
			if tc.hasStep {
				stepGetter.steps[stepID] = &wfmodels.WorkflowStep{ID: stepID, Prompt: durablePrompt}
			}
			var dispatched string
			agentMgr := &mockAgentManager{
				launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					dispatched = req.TaskDescription
					return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
				},
			}
			svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
			svc.promptExpander = prompts
			messages := &mockMessageCreator{}
			svc.messageCreator = messages

			_, err = svc.startTask(ctx, "task1", "profile1", "", "", "", "",
				stepID, false, true, nil, startTaskOptions{EntryOptions: &workflowmove.EntryOptions{
					Instructions: moveSavedPromptInstructions, SkipStepPrompt: tc.skipStep,
				}})
			require.NoError(t, err)
			require.Len(t, messages.userMessages, 1)
			require.Equal(t, messages.userMessages[0].content, dispatched)
			assertMoveSavedPromptDelivered(t, dispatched)
			if tc.hasStep && !tc.skipStep {
				require.Contains(t, dispatched, "Check the durable step policy.")
				require.Less(t, strings.Index(dispatched, durablePrompt), strings.Index(dispatched, workflowmove.InstructionsHeading))
			}
			if tc.skipStep {
				require.NotContains(t, dispatched, durablePrompt)
				require.NotContains(t, dispatched, "fallback description")
			} else if !tc.hasStep {
				require.Contains(t, dispatched, "fallback description")
			}
			if tc.hasStep {
				require.Equal(t, durablePrompt, stepGetter.steps[stepID].Prompt)
			}
		})
	}
}

// @covers AC-TASKS-SAVED-PROMPT-DELIVERY-001.17
func TestMoveInstructionSavedPromptQueuedHandoff(t *testing.T) {
	t.Run("managed", func(t *testing.T) {
		testMoveInstructionSavedPromptQueuedHandoff(t, false)
	})
	t.Run("persisted passthrough", func(t *testing.T) {
		testMoveInstructionSavedPromptQueuedHandoff(t, true)
	})
}

func testMoveInstructionSavedPromptQueuedHandoff(t *testing.T, persistedPassthrough bool) {
	t.Helper()
	ctx := context.Background()
	prompts := newPromptServiceForLaunchFallbackTest(t)
	_, err := prompts.CreatePrompt(ctx, "review-checklist", moveSavedPromptDefinition)
	require.NoError(t, err)
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	session.IsPassthrough = persistedPassthrough
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	agentMgr := &mockAgentManager{isAgentRunning: true, promptDone: make(chan struct{})}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageQueue = newSQLiteQueueForTaskRepo(t, repo)
	svc.executor = withTestCapabilities(executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}))
	svc.promptExpander = prompts
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	step := workflowmove.OverlayStep(&wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Review", Prompt: "Durable review instructions.",
	}, &workflowmove.EntryOptions{Instructions: moveSavedPromptInstructions})

	svc.processOnEnter(ctx, "t1", session, step, "task description", 0, nil)

	select {
	case <-agentMgr.promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for move instructions")
	}
	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	require.Len(t, agentMgr.capturedPrompts, 1)
	dispatched := agentMgr.capturedPrompts[0]
	require.Len(t, messages.userMessages, 1)
	require.Equal(t, messages.userMessages[0].content, dispatched)
	require.NotContains(t, dispatched, "Durable review instructions.")
	if persistedPassthrough {
		require.Equal(t, workflowmove.WrapInstructions(moveSavedPromptInstructions), dispatched)
		return
	}
	assertMoveSavedPromptDelivered(t, dispatched)
}

// @covers AC-TASKS-SAVED-PROMPT-DELIVERY-001.17
func TestMoveInstructionSavedPromptDeferred(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	prompts := newPromptServiceForLaunchFallbackTest(t)
	_, err := prompts.CreatePrompt(sc.ctx, "review-checklist", moveSavedPromptDefinition)
	require.NoError(t, err)
	sc.svc.promptExpander = prompts

	review, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	require.NoError(t, err)
	require.NoError(t, sc.svc.messageQueue.SetPendingMove(sc.ctx, sc.reviewSessionID, &messagequeue.PendingMove{
		SessionIncarnationID: review.QueueIncarnationID,
		TaskID:               review.TaskID,
		WorkflowID:           "wf1",
		FromStepID:           stepInReviewID,
		WorkflowStepID:       stepInProgressID,
		EntryOptions:         &workflowmove.EntryOptions{Instructions: moveSavedPromptInstructions},
	}))
	sc.svc.handleAgentReady(sc.ctx, watcher.AgentEventData{
		TaskID: review.TaskID, SessionID: review.ID,
		AgentExecutionID: "ae-review", AgentProfileID: profileReview,
	})

	var delivered []string
	require.Eventually(t, func() bool {
		delivered = capturedExecutionDescriptionsForExecution(sc.agentMgr, sc.implRelaunchExec)
		return len(delivered) > 0
	}, 5*time.Second, 20*time.Millisecond)
	require.Len(t, delivered, 1)
	assertMoveSavedPromptDelivered(t, delivered[0])
	task, err := sc.repo.GetTask(sc.ctx, review.TaskID)
	require.NoError(t, err)
	require.Equal(t, stepInProgressID, task.WorkflowStepID)
	require.NotContains(t, sc.stepGetter.steps[stepInProgressID].Prompt, moveSavedPromptInstructions)
}

// @covers AC-TASKS-SAVED-PROMPT-DELIVERY-001.18
func TestMoveInstructionSavedPromptPassthrough(t *testing.T) {
	ctx := context.Background()
	prompts := newPromptServiceForLaunchFallbackTest(t)
	_, err := prompts.CreatePrompt(ctx, "review-checklist", moveSavedPromptDefinition)
	require.NoError(t, err)
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.promptExpander = prompts

	prompt, _, trusted := svc.prepareFreshTaskPrompt(ctx, "", "task", "session", "",
		false, false, true, &workflowmove.EntryOptions{Instructions: moveSavedPromptInstructions})

	require.Equal(t, workflowmove.WrapInstructions(moveSavedPromptInstructions), prompt)
	require.Empty(t, trusted)
}

func TestMoveInstructionSavedPromptComposesBeforeReferenceResolution(t *testing.T) {
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-move-title", "wf-move-title", "step-review")
	setTaskTitle(t, repo, task, "Apply @title-guide")
	steps := newMockStepGetter()
	steps.steps[task.WorkflowStepID] = &wfmodels.WorkflowStep{
		ID: task.WorkflowStepID, Prompt: "{task_title}: {{task_prompt}}",
	}
	svc := createTestService(repo, steps, newMockTaskRepo())
	expander := &fakePromptReferenceExpander{}
	svc.promptExpander = expander

	_, _, trusted := svc.prepareFreshTaskPrompt(context.Background(), "Review the changes.", task.ID,
		"session", task.WorkflowStepID, true, false, false, &workflowmove.EntryOptions{
			Instructions: moveSavedPromptInstructions,
		})

	require.Equal(t, []string{"Apply @title-guide: Review the changes.\n\n" +
		workflowmove.WrapInstructions(moveSavedPromptInstructions)}, expander.calls)
	require.Equal(t, fakeResolvedPromptReferenceContext, trusted)
}

func assertMoveSavedPromptDelivered(t *testing.T, prompt string) {
	t.Helper()
	require.Contains(t, prompt, moveSavedPromptInstructions)
	require.Contains(t, prompt, moveSavedPromptDefinition)
	require.Equal(t, 1, strings.Count(prompt, workflowmove.InstructionsEnd))
	require.Equal(t, 1, strings.Count(prompt, "EXPANDED PROMPT REFERENCES:"))
}
