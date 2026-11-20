package promptsync

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
)

// keptVersionFixture leaves two synced prompts with kept local versions and
// one ordinary synced prompt.
func keptVersionFixture(t *testing.T) *syncFixture {
	t.Helper()
	f := newSyncFixture(t)
	ctx := context.Background()
	for _, name := range []string{"restore-me", "keep-repo"} {
		_, err := f.prompts.CreatePrompt(ctx, name, "Local "+name)
		require.NoError(t, err)
		f.gitlab.files[name+".md"] = "Repo " + name
	}
	f.gitlab.files["plain.md"] = "Repo plain"
	_, err := f.svc.SyncWithResolutions(ctx, map[string]promptservice.ConflictChoice{
		"restore-me": promptservice.ChoiceRepository, "keep-repo": promptservice.ChoiceRepository,
	})
	require.NoError(t, err)
	return f
}

func TestKeptVersionsListsSyncedPromptsWithLocalVersions(t *testing.T) {
	f := keptVersionFixture(t)
	kept, err := f.svc.KeptVersions(context.Background())
	require.NoError(t, err)
	require.Len(t, kept, 2)
	assert.Equal(t, Conflict{
		Name: "keep-repo", Path: DefaultPath + "/keep-repo.md",
		LocalContent: "Local keep-repo", RepositoryContent: "Repo keep-repo",
	}, kept[0])
	assert.Equal(t, "restore-me", kept[1].Name)
}

func TestDeleteConfigRestoresChosenLocalVersions(t *testing.T) {
	f := keptVersionFixture(t)
	require.NoError(t, f.svc.DeleteConfig(context.Background(), "restore-me"))
	restored := f.prompt(t, "restore-me")
	assert.Equal(t, promptmodels.PromptSourceManual, restored.Source)
	assert.Equal(t, "Local restore-me", restored.Content)
	assert.Equal(t, "Repo keep-repo", f.prompt(t, "keep-repo").Content)
	assert.Equal(t, "Repo plain", f.prompt(t, "plain").Content)
}

func TestHandlersListKeptVersionsAndRemoveWithRestore(t *testing.T) {
	f := keptVersionFixture(t)
	router := newTestRouter(t, f.svc, admin)

	listed := doRequest(router, http.MethodGet, "/api/v1/prompt-sync/kept-versions", "")
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var body struct {
		Prompts []Conflict `json:"prompts"`
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &body))
	assert.Len(t, body.Prompts, 2)

	removed := doRequest(router, http.MethodDelete, "/api/v1/prompt-sync/config", `{"restore_local":["restore-me"]}`)
	require.Equal(t, http.StatusOK, removed.Code, removed.Body.String())
	assert.Equal(t, "Local restore-me", f.prompt(t, "restore-me").Content)
	assert.Equal(t, "Repo keep-repo", f.prompt(t, "keep-repo").Content)
}
