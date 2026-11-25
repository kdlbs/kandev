package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplySyncedPromptsKeepsPromptsReachedThroughSurvivingPrompts(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "outer", Content: "Apply @inner", Path: "p/outer.md"},
		{Name: "inner", Content: "Apply @deepest", Path: "p/inner.md"},
		{Name: "deepest", Content: "D", Path: "p/deepest.md"},
		{Name: "used-by-manual", Content: "M", Path: "p/used-by-manual.md"},
		{Name: "orphan", Content: "O", Path: "p/orphan.md"},
	}})
	_, err := svc.CreatePrompt(ctx, "manual-rule", "See @used-by-manual")
	require.NoError(t, err)

	result := applySynced(t, svc, SyncApplyInput{
		ReferenceTexts: []PromptReferenceText{{Owner: "wf-1", Text: "Run @outer"}},
	})
	assert.Equal(t, []string{"orphan"}, result.Deleted)
	for _, name := range []string{"outer", "inner", "deepest", "used-by-manual"} {
		_, err := svc.GetPromptByName(ctx, name)
		assert.NoError(t, err, name)
	}
	assert.Len(t, result.Warnings, 4)
}
