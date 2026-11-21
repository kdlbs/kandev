package promptsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
)

func syncedPromptID(t *testing.T, f *syncFixture, name string) string {
	t.Helper()
	prompt, err := f.prompts.GetPromptByName(context.Background(), name)
	require.NoError(t, err)
	return prompt.ID
}

func TestSaveVersionsUsesTheStoredRepositoryVersionUntilTheNextSync(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["hook.md"] = "Default"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	id := syncedPromptID(t, f, "hook")

	local := "Local"
	overridden, err := f.svc.SaveVersions(ctx, id, promptservice.VersionSave{UseLocal: true, LocalContent: &local})
	require.NoError(t, err)
	assert.Equal(t, "Local", overridden.Content)

	f.gitlab.files["hook.md"] = "Default v2"
	result, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"hook"}, result.Overridden)

	f.gitlab.files["hook.md"] = "Default v3"
	reverted, err := f.svc.SaveVersions(ctx, id, promptservice.VersionSave{})
	require.NoError(t, err)
	assert.True(t, reverted.IsSynced())
	assert.Equal(t, "Default v2", reverted.Content, "choosing the repository version uses the stored version at once")
	assert.Equal(t, "Local", reverted.AlternateContent)

	_, err = f.svc.Sync(ctx)
	require.NoError(t, err)
	refreshed, err := f.prompts.GetPromptByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Default v3", refreshed.Content, "the next sync refreshes the repository version")
	assert.Equal(t, "Local", refreshed.AlternateContent)
}

func TestSaveVersionsAuthorizesTheCredentialWorkspace(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["hook.md"] = "Default"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	denied := errors.New("denied")
	f.svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return denied })
	_, err = f.svc.SaveVersions(ctx, syncedPromptID(t, f, "hook"), promptservice.VersionSave{UseLocal: true})
	assert.ErrorIs(t, err, denied)
}

func TestSaveVersionsRoute(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["hook.md"] = "Default"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	id := syncedPromptID(t, f, "hook")
	manual, err := f.prompts.CreatePrompt(ctx, "manual", "Mine")
	require.NoError(t, err)
	path := "/api/v1/prompt-sync/prompts/" + id + "/versions"

	member := newTestRouter(t, f.svc, authn.Identity{UserID: "m", Role: authn.RoleMember, OrgID: "org-1"})
	assert.Equal(t, http.StatusForbidden, doRequest(member, http.MethodPut, path, `{"use_local":true}`).Code)

	router := newTestRouter(t, f.svc, admin)
	saved := doRequest(router, http.MethodPut, path, `{"use_local":true,"local_content":"Edited"}`)
	require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
	var body struct {
		Source           string `json:"source"`
		Content          string `json:"content"`
		AlternateContent string `json:"alternate_content"`
	}
	require.NoError(t, json.Unmarshal(saved.Body.Bytes(), &body))
	assert.Equal(t, "override", body.Source)
	assert.Equal(t, "Edited", body.Content)
	assert.Equal(t, "Default", body.AlternateContent)

	stale := doRequest(router, http.MethodPut, path, `{"use_local":false,"expected_updated_at":"2001-01-01T00:00:00Z"}`)
	assert.Equal(t, http.StatusConflict, stale.Code, "a save based on an obsolete revision is rejected")
	assert.Equal(t, http.StatusBadRequest, doRequest(router, http.MethodPut, path, `{"use_local":true,"local_content":" "}`).Code)
	assert.Equal(t, http.StatusBadRequest, doRequest(router, http.MethodPut, path, `not json`).Code)
	assert.Equal(t, http.StatusConflict, doRequest(router, http.MethodPut, "/api/v1/prompt-sync/prompts/"+manual.ID+"/versions", `{"use_local":true}`).Code)
	assert.Equal(t, http.StatusNotFound, doRequest(router, http.MethodPut, "/api/v1/prompt-sync/prompts/missing/versions", `{}`).Code)
	assert.Equal(t, http.StatusNotFound, doRequest(router, http.MethodPost, "/api/v1/prompt-sync/prompts/"+id+"/override", "").Code, "the old override route is gone")
}

func TestSaveVersionsRouteRejectsRepositoryWhenTheFileIsGone(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["hook.md"] = "Default"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	id := syncedPromptID(t, f, "hook")
	router := newTestRouter(t, f.svc, admin)
	path := "/api/v1/prompt-sync/prompts/" + id + "/versions"
	require.Equal(t, http.StatusOK, doRequest(router, http.MethodPut, path, `{"use_local":true,"local_content":"Mine"}`).Code)
	delete(f.gitlab.files, "hook.md")
	_, err = f.svc.Sync(ctx)
	require.NoError(t, err)

	rejected := doRequest(router, http.MethodPut, path, `{"use_local":false}`)
	assert.Equal(t, http.StatusConflict, rejected.Code, rejected.Body.String())
	prompt, err := f.prompts.GetPromptByName(ctx, "hook")
	require.NoError(t, err, "the only copy is never deleted")
	assert.Equal(t, "Mine", prompt.Content)
}
