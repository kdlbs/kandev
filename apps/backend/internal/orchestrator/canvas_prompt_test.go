package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
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
