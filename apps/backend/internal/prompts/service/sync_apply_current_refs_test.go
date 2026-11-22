package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplySyncedPromptsKeepsPromptsReferencedByThisRunsWrites(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "outer", Content: "No references yet", Path: "p/outer.md"},
		{Name: "inner", Content: "Inner rule", Path: "p/inner.md"},
		{Name: "helper", Content: "Helper rule", Path: "p/helper.md"},
		{Name: "orphan", Content: "Orphan rule", Path: "p/orphan.md"},
	}})

	result := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "outer", Content: "Apply @inner", Path: "p/outer.md"},
		{Name: "fresh", Content: "Use @helper", Path: "p/fresh.md"},
	}})

	assert.Equal(t, []string{"orphan"}, result.Deleted)
	for _, name := range []string{"inner", "helper"} {
		_, err := svc.GetPromptByName(ctx, name)
		assert.NoError(t, err, "%s is referenced by a prompt this run wrote", name)
	}
	require.Len(t, result.Warnings, 2)
}
