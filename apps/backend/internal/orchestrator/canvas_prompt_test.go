package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapCreatedSessionPrompt_CanvasPromptFollowsResolvedCapability(t *testing.T) {
	session := &models.TaskSession{ID: "session"}
	task := &models.Task{ID: "task"}

	withoutCanvas := (&Service{}).wrapCreatedSessionPrompt(
		context.Background(), "build the requested app", "task", "session", session, task,
		false, false, false, false, nil, "",
	)
	assert.NotContains(t, withoutCanvas, "create_canvas_kandev")

	withCanvas := (&Service{}).wrapCreatedSessionPrompt(
		context.Background(), "build the requested app", "task", "session", session, task,
		false, false, false, true, nil, "",
	)
	assertCanvasPrompt(t, withCanvas)
}

func TestApplyLaunchPromptContext_CanvasPromptFollowsResolvedCapability(t *testing.T) {
	withoutCanvas := (&Service{}).applyLaunchPromptContext(context.Background(), launchPromptContext{
		prompt:    "build the requested app",
		taskID:    "task",
		sessionID: "session",
	})
	assert.NotContains(t, withoutCanvas, "create_canvas_kandev")

	withCanvas := (&Service{}).applyLaunchPromptContext(context.Background(), launchPromptContext{
		prompt:                "build the requested app",
		taskID:                "task",
		sessionID:             "session",
		includeCanvasGuidance: true,
	})
	assertCanvasPrompt(t, withCanvas)
	assert.Contains(t, withCanvas, "build the requested app")
	assert.Equal(t, 1, countSystemBlocks(withCanvas))

	assert.NotContains(t, sysprompt.StripSystemContent(withCanvas), "create_canvas_kandev")
}

func TestTaskSessionCanvasGuidanceEnabledRejectsMismatchedPair(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-a", "session-a", models.TaskSessionStateCreated)
	seedTaskAndSession(t, repo, "task-b", "session-b", models.TaskSessionStateCreated)

	_, err := (&Service{repo: repo}).TaskSessionCanvasGuidanceEnabled(context.Background(), "task-a", "session-b")
	require.ErrorIs(t, err, ErrTaskSessionPairMismatch)
}

// @covers AC-AGENTS-MCP-DISCOVERY-002.2
func TestStartCreatedSession_UsesResolvedCanvasCapabilityForRecordedPrompt(t *testing.T) {
	for _, tt := range []struct {
		name       string
		canvases   bool
		wantCanvas bool
	}{
		{name: "canvas enabled", canvases: true, wantCanvas: true},
		{name: "canvas disabled", canvases: false, wantCanvas: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateCreated)
			seedExecutorRunning(t, repo, "session", "task", "exec-1")

			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task"] = &v1.Task{
				ID: "task", Title: "Build a canvas", Description: "Build the requested app",
				State: v1.TaskStateInProgress,
			}
			agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
			svc.SetCanvasesEnabled(tt.canvases)
			messages := &mockMessageCreator{}
			svc.messageCreator = messages

			_, err := svc.StartCreatedSession(
				ctx, "task", "session", "profile-1", "Build the requested app",
				false, false, false, nil, nil,
			)
			require.NoError(t, err)
			require.Len(t, messages.userMessages, 1)
			agentMgr.mu.Lock()
			descriptions := append([]promptCall(nil), agentMgr.setExecutionDescriptionCalls...)
			agentMgr.mu.Unlock()
			require.Len(t, descriptions, 1)
			if tt.wantCanvas {
				assertCanvasPrompt(t, messages.userMessages[0].content)
				assertCanvasPrompt(t, descriptions[0].Prompt)
			} else {
				assert.NotContains(t, messages.userMessages[0].content, "create_canvas_kandev")
				assert.NotContains(t, descriptions[0].Prompt, "create_canvas_kandev")
			}
		})
	}
}

// @covers AC-AGENTS-MCP-DISCOVERY-002.2
func TestStartTask_UsesResolvedCanvasCapabilityForRecordedAndDispatchedPrompt(t *testing.T) {
	for _, tt := range []struct {
		name       string
		canvases   bool
		wantCanvas bool
	}{
		{name: "canvas enabled", canvases: true, wantCanvas: true},
		{name: "canvas disabled", canvases: false, wantCanvas: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task", "completed-session", models.TaskSessionStateCompleted)

			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task"] = &v1.Task{
				ID: "task", Title: "Build a canvas", Description: "Build the requested app",
				State: v1.TaskStateInProgress,
			}
			var dispatchedPrompt string
			agentMgr := &mockAgentManager{
				launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					dispatchedPrompt = req.TaskDescription
					return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
				},
			}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
			svc.SetCanvasesEnabled(tt.canvases)
			messages := &mockMessageCreator{}
			svc.messageCreator = messages

			_, err := svc.StartTask(
				ctx, "task", "profile-1", "", "", "", "Build the requested app",
				"", false, false, nil,
			)
			require.NoError(t, err)
			require.Len(t, messages.userMessages, 1)
			if tt.wantCanvas {
				assertCanvasPrompt(t, messages.userMessages[0].content)
				assertCanvasPrompt(t, dispatchedPrompt)
			} else {
				assert.NotContains(t, messages.userMessages[0].content, "create_canvas_kandev")
				assert.NotContains(t, dispatchedPrompt, "create_canvas_kandev")
			}
			assert.Contains(t, messages.userMessages[0].content, "Build the requested app")
			assert.Contains(t, dispatchedPrompt, "Build the requested app")
		})
	}
}

func countSystemBlocks(prompt string) int {
	return strings.Count(prompt, sysprompt.TagStart)
}

func assertCanvasPrompt(t *testing.T, prompt string) {
	t.Helper()
	for _, tool := range []string{
		"create_canvas_kandev",
		"read_canvas_authoring_skill_kandev",
		"publish_canvas_kandev",
	} {
		assert.Contains(t, prompt, tool)
	}
}
