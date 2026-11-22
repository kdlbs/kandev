package promptsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	promptservice "github.com/kandev/kandev/internal/prompts/service"
)

// A renamed override file is a removal plus a new file. The override keeps the
// old name with the user's text, and the new name becomes a synced prompt.
func TestRenamedOverrideFileKeepsOldOverrideAndCreatesSyncedPrompt(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	_, err := f.prompts.CreatePrompt(ctx, "keep", "Local keep")
	require.NoError(t, err)
	f.gitlab.files["keep.md"] = "Repo keep"
	_, err = f.svc.SyncWithResolutions(ctx, map[string]promptservice.ConflictChoice{"keep": promptservice.ChoiceLocal})
	require.NoError(t, err)
	require.True(t, f.prompt(t, "keep").IsOverride())

	delete(f.gitlab.files, "keep.md")
	f.gitlab.files["kept.md"] = "Repo keep"
	_, err = f.svc.Sync(ctx)
	require.NoError(t, err)

	old := f.prompt(t, "keep")
	assert.True(t, old.IsOverride(), "the override keeps its ownership")
	assert.Equal(t, "Local keep", old.Content, "the user's text stays in use")
	assert.Empty(t, old.AlternateContent, "no repository version is left for a removed file")

	created := f.prompt(t, "kept")
	assert.True(t, created.IsSynced())
	assert.Equal(t, "Repo keep", created.Content)
}
