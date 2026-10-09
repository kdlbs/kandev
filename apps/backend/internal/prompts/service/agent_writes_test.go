package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/prompts/models"
	"github.com/stretchr/testify/require"
)

type agentPromptWriter interface {
	CreatePromptForAgent(context.Context, string, string) (*models.Prompt, error)
	UpdatePromptContentForAgent(context.Context, string, string) (*models.Prompt, error)
	UpdatePromptForAgent(context.Context, string, *string, *string) (*models.Prompt, error)
	UpdatePromptWithPermission(context.Context, string, *string, *string, *bool) (*models.Prompt, error)
}

// @covers AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.2
// @covers AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.4
func TestAgentPromptWrites(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	writer, ok := any(svc).(agentPromptWriter)
	require.True(t, ok, "prompt service must support guarded agent writes")
	ctx := context.Background()
	created, err := writer.CreatePromptForAgent(ctx, " review ", " Original ")
	require.NoError(t, err)
	require.Equal(t, "review", created.Name)
	updated, err := writer.UpdatePromptContentForAgent(ctx, " review ", " Revised ")
	require.NoError(t, err)
	require.Equal(t, created.ID, updated.ID)
	require.Equal(t, "Revised", updated.Content)
	_, err = writer.CreatePromptForAgent(ctx, "review", "duplicate")
	require.ErrorIs(t, err, ErrPromptAlreadyExists)
	_, err = writer.CreatePromptForAgent(ctx, "code-review", "duplicate builtin")
	require.ErrorIs(t, err, ErrPromptAlreadyExists)
	_, err = writer.UpdatePromptContentForAgent(ctx, "REVIEW", "wrong case")
	require.ErrorIs(t, err, ErrPromptNotFound)
	_, err = writer.UpdatePromptContentForAgent(ctx, "review", " ")
	require.ErrorIs(t, err, ErrInvalidPrompt)
	human, err := svc.CreatePrompt(ctx, "human", "Human content")
	require.NoError(t, err)
	_, err = writer.UpdatePromptContentForAgent(ctx, human.Name, "Agent content")
	require.ErrorContains(t, err, "agent edits")
	allowed := true
	_, err = writer.UpdatePromptWithPermission(ctx, human.ID, nil, nil, &allowed)
	require.NoError(t, err)
	updated, err = writer.UpdatePromptContentForAgent(ctx, human.Name, "Allowed content")
	require.NoError(t, err)
	require.Equal(t, "Allowed content", updated.Content)
	allowed = false
	_, err = writer.UpdatePromptWithPermission(ctx, human.ID, nil, nil, &allowed)
	require.NoError(t, err)
	changed := "Bypass attempt"
	_, err = writer.UpdatePromptForAgent(ctx, human.ID, nil, &changed)
	require.ErrorContains(t, err, "agent edits")
	builtin, err := svc.GetPromptByName(ctx, "code-review")
	require.NoError(t, err)
	_, err = writer.UpdatePromptContentForAgent(ctx, builtin.Name, "Agent content")
	require.ErrorIs(t, err, ErrBuiltinPrompt)
	persisted, err := svc.GetPromptByName(ctx, builtin.Name)
	require.NoError(t, err)
	require.Equal(t, builtin.Content, persisted.Content)
	expansions, err := svc.ResolvePromptReferences(ctx, "Apply @review")
	require.NoError(t, err)
	require.Equal(t, []PromptReferenceExpansion{{Name: "review", Content: "Revised"}}, expansions)
}
