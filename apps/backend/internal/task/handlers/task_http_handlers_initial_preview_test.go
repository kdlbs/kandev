package handlers

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PROMPT-ATTACHMENTS-001.8
func TestTaskCreateInitialPreviewReachesPreparation(t *testing.T) {
	orch := &captureOrchestrator{prepErr: errors.New("stop before async dispatch")}
	h := &TaskHandlers{orchestrator: orch, logger: newTestLogger(t)}
	body := httpCreateTaskRequest{
		StartAgent: true, AgentProfileID: "profile-1", Description: "submitted text",
		Attachments: []v1.MessageAttachment{{AttachmentID: "image-1", Type: "image", MimeType: "image/png", Name: "screen.png"}},
	}
	h.prepareStartAgentSession(context.Background(), &createTaskResponse{}, "task-1", body, "step-1")
	require.Len(t, orch.requests, 1)
	preview := orch.requests[0].InitialPromptPreview
	require.NotNil(t, preview)
	require.Equal(t, body.Description, preview.Content)
	require.Equal(t, body.Attachments, preview.Attachments)
	require.Empty(t, orch.requests[0].Prompt, "display text must not dispatch an agent prompt")
	require.Empty(t, orch.requests[0].Attachments)
}

func TestTaskCreateInitialPreviewPrepareOnly(t *testing.T) {
	orch := &captureOrchestrator{prepErr: errors.New("stop before workspace launch")}
	h := &TaskHandlers{orchestrator: orch, logger: newTestLogger(t)}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/tasks", nil)
	body := httpCreateTaskRequest{
		PrepareSession: true, AgentProfileID: "profile-1", Description: "submitted text",
		Attachments: []v1.MessageAttachment{{AttachmentID: "file-1", Type: "resource", Name: "notes.txt", MimeType: "text/plain"}},
	}
	h.prepareTaskSession(c, &createTaskResponse{}, "task-1", body, "step-1", true)
	require.Len(t, orch.requests, 1)
	require.Equal(t, body.Attachments, orch.requests[0].InitialPromptPreview.Attachments)
	require.True(t, orch.requests[0].LaunchWorkspace)
	require.False(t, orch.requests[0].DeferredStart)
}
