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

// twoVersionFixture leaves prompts that have both versions in each way they
// can get there, plus an ordinary synced prompt and an unedited override:
//   - taken: a manual prompt where the user chose the repository version;
//   - kept: a manual prompt where the user kept the local version;
//   - edited: a synced prompt the user overrode and then edited.
func twoVersionFixture(t *testing.T) *syncFixture {
	t.Helper()
	f := newSyncFixture(t)
	ctx := context.Background()
	for _, name := range []string{"taken", "kept"} {
		_, err := f.prompts.CreatePrompt(ctx, name, "Local "+name)
		require.NoError(t, err)
		f.gitlab.files[name+".md"] = "Repo " + name
	}
	for _, name := range []string{"edited", "plain", "untouched"} {
		f.gitlab.files[name+".md"] = "Repo " + name
	}
	_, err := f.svc.SyncWithResolutions(ctx, map[string]promptservice.ConflictChoice{
		"taken": promptservice.ChoiceRepository, "kept": promptservice.ChoiceLocal,
	})
	require.NoError(t, err)
	_, err = f.prompts.SavePromptVersions(ctx, f.prompt(t, "untouched").ID, promptservice.VersionSave{UseLocal: true})
	require.NoError(t, err)
	local := "Local edited"
	_, err = f.prompts.SavePromptVersions(ctx, f.prompt(t, "edited").ID, promptservice.VersionSave{UseLocal: true, LocalContent: &local})
	require.NoError(t, err)
	return f
}

func TestRemovalReviewListsEveryPromptWithDifferentVersions(t *testing.T) {
	f := twoVersionFixture(t)
	review, err := f.svc.RemovalReview(context.Background())
	require.NoError(t, err)
	listed := review.Prompts
	assert.Equal(t, []string{"plain"}, review.RepositoryOnly, "only plain came from the repository alone")
	byName := map[string]Conflict{}
	for _, item := range listed {
		byName[item.Name] = item
	}
	assert.Len(t, listed, 3, "plain and the unedited override have one version")
	assert.Equal(t, Conflict{
		Name: "edited", Path: DefaultPath + "/edited.md", LocalContent: "Local edited",
		RepositoryContent: "Repo edited", InUse: promptservice.ChoiceLocal,
	}, byName["edited"])
	assert.Equal(t, promptservice.ChoiceLocal, byName["kept"].InUse)
	assert.Equal(t, "Local taken", byName["taken"].LocalContent)
	assert.Equal(t, promptservice.ChoiceRepository, byName["taken"].InUse)
}

func TestDeleteConfigAppliesVersionChoices(t *testing.T) {
	f := twoVersionFixture(t)
	require.NoError(t, f.svc.DeleteConfig(context.Background(), Removal{Choices: map[string]promptservice.ConflictChoice{
		"taken": promptservice.ChoiceLocal, "edited": promptservice.ChoiceRepository, "kept": promptservice.ChoiceLocal,
	}}))
	for name, want := range map[string]string{
		"taken": "Local taken", "edited": "Repo edited", "kept": "Local kept", "plain": "Repo plain",
	} {
		prompt := f.prompt(t, name)
		assert.Equal(t, promptmodels.PromptSourceManual, prompt.Source, name)
		assert.Equal(t, want, prompt.Content, name)
	}
}

func TestHandlersListTwoVersionPromptsAndRemoveWithChoices(t *testing.T) {
	f := twoVersionFixture(t)
	router := newTestRouter(t, f.svc, admin)

	listed := doRequest(router, http.MethodGet, "/api/v1/prompt-sync/kept-versions", "")
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var body struct {
		Prompts        []Conflict `json:"prompts"`
		RepositoryOnly []string   `json:"repository_only"`
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &body))
	assert.Len(t, body.Prompts, 3)
	assert.Equal(t, []string{"plain"}, body.RepositoryOnly)

	invalid := doRequest(router, http.MethodDelete, "/api/v1/prompt-sync/config", `{"choices":{"taken":"both"}}`)
	assert.Equal(t, http.StatusBadRequest, invalid.Code)

	removed := doRequest(router, http.MethodDelete, "/api/v1/prompt-sync/config", `{"choices":{"edited":"repository"}}`)
	require.Equal(t, http.StatusOK, removed.Code, removed.Body.String())
	assert.Equal(t, "Repo edited", f.prompt(t, "edited").Content)
	assert.Equal(t, "Repo taken", f.prompt(t, "taken").Content, "without a choice the version in use stays")
}

func TestDeleteConfigCanDeleteRepositoryOnlyPrompts(t *testing.T) {
	f := twoVersionFixture(t)
	router := newTestRouter(t, f.svc, admin)
	removed := doRequest(router, http.MethodDelete, "/api/v1/prompt-sync/config", `{"delete_repository_only":["plain"]}`)
	require.Equal(t, http.StatusOK, removed.Code, removed.Body.String())

	_, err := f.prompts.GetPromptByName(context.Background(), "plain")
	assert.ErrorIs(t, err, promptservice.ErrPromptNotFound, "a prompt that only came from the repository is deleted")
	for _, name := range []string{"taken", "kept", "edited", "untouched"} {
		assert.Equal(t, promptmodels.PromptSourceManual, f.prompt(t, name).Source, name)
	}
}

func TestDeleteConfigKeepsRepositoryOnlyPromptsTheUserDidNotReview(t *testing.T) {
	f := twoVersionFixture(t)
	router := newTestRouter(t, f.svc, admin)
	removed := doRequest(router, http.MethodDelete, "/api/v1/prompt-sync/config", `{"delete_repository_only":["missing"]}`)
	require.Equal(t, http.StatusOK, removed.Code, removed.Body.String())
	assert.Equal(t, promptmodels.PromptSourceManual, f.prompt(t, "plain").Source, "an unreviewed prompt is released, not deleted")
}
