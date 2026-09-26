package service

import (
	"context"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetWorkflowMeta_ReturnsProfileAndPromptInOneRead(t *testing.T) {
	svc, _ := setupTestService(t)
	provider := &mockWorkflowProvider{}
	now := time.Now().UTC()
	provider.workflows = append(provider.workflows, &taskmodels.Workflow{
		ID:             "wf-meta",
		WorkspaceID:    "ws-1",
		Name:           "Meta",
		AgentProfileID: "profile-42",
		Prompt:         "Always keep CI green.",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	svc.SetWorkflowProvider(provider)

	meta, err := svc.GetWorkflowMeta(context.Background(), "wf-meta")
	require.NoError(t, err)
	assert.Equal(t, "profile-42", meta.AgentProfileID)
	assert.Equal(t, "Always keep CI green.", meta.Prompt)
	assert.Equal(t, 1, provider.getWorkflowCalls)

	// Thin wrappers share the batched path but each call still hits the provider
	// once when no request cache is present (as expected outside orchestrator).
	profileID, err := svc.GetWorkflowAgentProfileID(context.Background(), "wf-meta")
	require.NoError(t, err)
	assert.Equal(t, "profile-42", profileID)

	prompt, err := svc.GetWorkflowPrompt(context.Background(), "wf-meta")
	require.NoError(t, err)
	assert.Equal(t, "Always keep CI green.", prompt)
	assert.Equal(t, 3, provider.getWorkflowCalls)
}

func TestGetWorkflowMeta_PropagatesMissingWorkflow(t *testing.T) {
	svc, _ := setupTestService(t)
	provider := &mockWorkflowProvider{}
	svc.SetWorkflowProvider(provider)

	_, err := svc.GetWorkflowMeta(context.Background(), "missing")
	require.Error(t, err)
	assert.Equal(t, 1, provider.getWorkflowCalls)
}
