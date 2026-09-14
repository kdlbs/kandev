package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const canvasTaskPrompt = "Build a coordinator canvas.\n\n@create-canvas"

// @covers AC-CANVASES-AGENT-WEB-APPS-009.8, AC-CANVASES-AGENT-WEB-APPS-009.9
func TestCreateCanvasPromptLaunch(t *testing.T) {
	t.Run("new task start expands the current built-in once", func(t *testing.T) {
		ctx := context.Background()
		promptService := newPromptServiceForLaunchFallbackTest(t)
		canvasDefinition := requireCreateCanvasDefinition(t, ctx, promptService)
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, "task-new", "completed-session", models.TaskSessionStateCompleted)

		taskRepo := newMockTaskRepo()
		taskRepo.tasks["task-new"] = &v1.Task{
			ID: "task-new", Title: "Canvas task", Description: canvasTaskPrompt, State: v1.TaskStateInProgress,
		}
		var dispatchedPrompt string
		agentManager := &mockAgentManager{
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				dispatchedPrompt = req.TaskDescription
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-new"}, nil
			},
		}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
		svc.promptExpander = promptService
		messages := &mockMessageCreator{}
		svc.messageCreator = messages

		_, err := svc.StartTask(ctx, "task-new", "profile-1", "", "", "", canvasTaskPrompt, "", false, false, nil)
		require.NoError(t, err)
		require.Len(t, messages.userMessages, 1)
		require.NotEmpty(t, dispatchedPrompt)
		assertCreateCanvasExpansion(t, messages.userMessages[0].content, canvasDefinition)
		assertCreateCanvasExpansion(t, dispatchedPrompt, canvasDefinition)
	})

	t.Run("prepared launch resolves the definition that exists at launch", func(t *testing.T) {
		ctx := context.Background()
		promptService := newPromptServiceForLaunchFallbackTest(t)
		canvasPrompt := requireCreateCanvasDefinition(t, ctx, promptService)
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, "task-prepared", "session-prepared", models.TaskSessionStateCreated)
		seedExecutorRunning(t, repo, "session-prepared", "task-prepared", "exec-prepared")
		before, err := repo.ListMessages(ctx, "session-prepared")
		require.NoError(t, err)
		require.Empty(t, before)

		updatedDefinition := canvasPrompt + "\n\nUse the current workspace task list."
		prompt, err := promptService.GetPromptByName(ctx, "create-canvas")
		require.NoError(t, err)
		_, err = promptService.UpdatePrompt(ctx, prompt.ID, nil, &updatedDefinition)
		require.NoError(t, err)

		taskRepo := newMockTaskRepo()
		taskRepo.tasks["task-prepared"] = &v1.Task{
			ID: "task-prepared", Title: "Canvas task", Description: canvasTaskPrompt, State: v1.TaskStateInProgress,
		}
		agentManager := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
		svc.promptExpander = promptService
		messages := &mockMessageCreator{}
		svc.messageCreator = messages

		_, err = svc.StartCreatedSession(ctx, "task-prepared", "session-prepared", "profile-1", canvasTaskPrompt, false, false, false, nil, nil)
		require.NoError(t, err)
		require.Len(t, messages.userMessages, 1)
		assertCreateCanvasExpansion(t, messages.userMessages[0].content, updatedDefinition)
	})

	t.Run("workflow step retains the short request and expands once", func(t *testing.T) {
		ctx := context.Background()
		promptService := newPromptServiceForLaunchFallbackTest(t)
		canvasDefinition := requireCreateCanvasDefinition(t, ctx, promptService)
		repo := setupTestRepo(t)
		seedTaskAndSessionWithStep(t, repo, "task-workflow", "session-workflow", "step-canvas")
		seedExecutorRunning(t, repo, "session-workflow", "task-workflow", "exec-workflow")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step-canvas"] = &wfmodels.WorkflowStep{
			ID: "step-canvas", WorkflowID: "wf1", Prompt: "Follow the task request:\n\n{{task_prompt}}",
		}
		taskRepo := newMockTaskRepo()
		taskRepo.tasks["task-workflow"] = &v1.Task{
			ID: "task-workflow", Title: "Canvas task", Description: canvasTaskPrompt, State: v1.TaskStateInProgress,
		}
		agentManager := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
		svc.promptExpander = promptService
		messages := &mockMessageCreator{}
		svc.messageCreator = messages

		_, err := svc.StartCreatedSession(ctx, "task-workflow", "session-workflow", "profile-1", canvasTaskPrompt, false, false, false, nil, nil)
		require.NoError(t, err)
		require.Len(t, messages.userMessages, 1)
		got := messages.userMessages[0].content
		require.Contains(t, got, canvasTaskPrompt)
		assertCreateCanvasExpansion(t, got, canvasDefinition)
	})

	t.Run("removed reference has no hidden expansion", func(t *testing.T) {
		ctx := context.Background()
		promptService := newPromptServiceForLaunchFallbackTest(t)
		canvasDefinition := requireCreateCanvasDefinition(t, ctx, promptService)
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, "task-plain", "session-plain", models.TaskSessionStateCreated)
		seedExecutorRunning(t, repo, "session-plain", "task-plain", "exec-plain")
		taskRepo := newMockTaskRepo()
		taskRepo.tasks["task-plain"] = &v1.Task{
			ID: "task-plain", Title: "Plain task", Description: "Build a coordinator canvas.", State: v1.TaskStateInProgress,
		}
		agentManager := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
		svc.promptExpander = promptService
		messages := &mockMessageCreator{}
		svc.messageCreator = messages

		_, err := svc.StartCreatedSession(ctx, "task-plain", "session-plain", "profile-1", "Build a coordinator canvas.", false, false, false, nil, nil)
		require.NoError(t, err)
		require.Len(t, messages.userMessages, 1)
		got := messages.userMessages[0].content
		require.NotContains(t, got, canvasDefinition)
		require.NotContains(t, got, "### @create-canvas")
	})
}

func requireCreateCanvasDefinition(t *testing.T, ctx context.Context, service *promptservice.Service) string {
	t.Helper()
	definition := service.ResolvePromptContent(ctx, "create-canvas", "")
	require.NotEmpty(t, definition)
	return definition
}

func assertCreateCanvasExpansion(t *testing.T, prompt, definition string) {
	t.Helper()
	require.Contains(t, prompt, canvasTaskPrompt)
	require.Contains(t, prompt, definition)
	require.Equal(t, 1, strings.Count(prompt, "### @create-canvas"))
	require.Equal(t, 1, strings.Count(prompt, definition))
}
