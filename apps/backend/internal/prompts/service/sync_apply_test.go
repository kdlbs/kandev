package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

func subscribePromptEvents(t *testing.T, svc *Service) *[]*bus.Event {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc.SetEventBus(eventBus, log)
	received := &[]*bus.Event{}
	_, err = eventBus.Subscribe("prompts.changed", func(_ context.Context, event *bus.Event) error {
		*received = append(*received, event)
		return nil
	})
	require.NoError(t, err)
	return received
}

func applySynced(t *testing.T, svc *Service, input SyncApplyInput) *SyncApplyResult {
	t.Helper()
	result, err := svc.ApplySyncedPrompts(context.Background(), input)
	require.NoError(t, err)
	return result
}

func TestApplySyncedPromptsCreatesUpdatesAndSkipsUnchanged(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	events := subscribePromptEvents(t, svc)
	ctx := context.Background()

	result := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "principles", Content: "Be simple", Path: "p/principles.md"},
	}})
	assert.Equal(t, []string{"principles"}, result.Created)
	require.Len(t, *events, 1)
	prompt, err := svc.GetPromptByName(ctx, "principles")
	require.NoError(t, err)
	assert.True(t, prompt.IsSynced())
	assert.Equal(t, "p/principles.md", prompt.SourcePath)

	result = applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "principles", Content: "Be simple", Path: "p/principles.md"},
	}})
	assert.Empty(t, result.Created)
	assert.Empty(t, result.Updated)
	assert.Len(t, *events, 1, "an unchanged sync must not publish")

	result = applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "principles", Content: "Be simpler", Path: "p/principles.md"},
	}})
	assert.Equal(t, []string{"principles"}, result.Updated)
	assert.Len(t, *events, 2)
	prompt, err = svc.GetPromptByName(ctx, "principles")
	require.NoError(t, err)
	assert.Equal(t, "Be simpler", prompt.Content)
}

func TestApplySyncedPromptsRemovalRespectsFrozenAndReferences(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "frozen", Content: "F", Path: "p/frozen.md"},
		{Name: "review", Content: "R", Path: "p/review.md"},
		{Name: "review-strict", Content: "RS", Path: "p/review-strict.md"},
		{Name: "unused", Content: "U", Path: "p/unused.md"},
	}})

	result := applySynced(t, svc, SyncApplyInput{
		FrozenNames: map[string]bool{"frozen": true},
		ReferenceTexts: []PromptReferenceText{
			{Owner: "wf-1", Text: "Run @review-strict now"},
			{Owner: "wf-2", Text: "Then @review-strict"},
			{Owner: "wf-2", Text: "and again @review-strict"},
		},
	})
	assert.Equal(t, []string{"review", "unused"}, result.Deleted)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], `"review-strict"`)
	assert.Contains(t, result.Warnings[0], "2 workflow")

	for _, name := range []string{"frozen", "review-strict"} {
		_, err := svc.GetPromptByName(ctx, name)
		assert.NoError(t, err, name)
	}
	_, err := svc.GetPromptByName(ctx, "unused")
	assert.ErrorIs(t, err, ErrPromptNotFound)
}

func TestSyncedPromptsAreReadOnlyToOperatorsAndAgents(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "locked", Content: "Repo", Path: "p/locked.md"}}})
	prompt, err := svc.GetPromptByName(ctx, "locked")
	require.NoError(t, err)

	edit := "edited"
	_, err = svc.UpdatePrompt(ctx, prompt.ID, nil, &edit)
	assert.ErrorIs(t, err, ErrPromptSyncManaged)
	assert.ErrorIs(t, svc.DeletePrompt(ctx, prompt.ID), ErrPromptSyncManaged)
	_, err = svc.UpdatePromptContentForAgent(ctx, "locked", edit)
	assert.ErrorIs(t, err, ErrPromptSyncManaged)
	_, err = svc.UpdatePromptForAgent(ctx, prompt.ID, nil, &edit)
	assert.ErrorIs(t, err, ErrPromptSyncManaged)
	_, err = svc.CreatePrompt(ctx, "locked", "dup")
	assert.ErrorIs(t, err, ErrPromptAlreadyExists)

	after, err := svc.GetPromptByID(ctx, prompt.ID)
	require.NoError(t, err)
	assert.Equal(t, "Repo", after.Content)

	released, err := svc.ReleaseSyncedPrompts(ctx, promptstore.ReleaseOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), released)
	_, err = svc.UpdatePrompt(ctx, prompt.ID, nil, &edit)
	assert.NoError(t, err)
}

func TestUnresolvedPromptReferences(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	_, err := svc.CreatePrompt(ctx, "principles", "Be simple")
	require.NoError(t, err)

	missing, err := svc.UnresolvedPromptReferences(ctx, []string{
		"Apply @principles and @missing-rule, then @missing-rule again.",
		"Mail me at a@b.example; no refs here",
		"@code-review @ghost",
	})
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"missing-rule"}, nil, {"ghost"}}, missing)
}
