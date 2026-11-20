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

func TestOverrideThenRevertRestoresTheRepositoryVersion(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["hook.md"] = "Default"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	id := syncedPromptID(t, f, "hook")

	_, err = f.svc.Override(ctx, id)
	require.NoError(t, err)
	local := "Local"
	_, err = f.prompts.UpdatePrompt(ctx, id, nil, &local)
	require.NoError(t, err)

	f.gitlab.files["hook.md"] = "Default v2"
	result, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"hook"}, result.Overridden)
	assert.Empty(t, result.Warnings)

	reverted, err := f.svc.Revert(ctx, id)
	require.NoError(t, err)
	assert.True(t, reverted.IsSynced())
	assert.Equal(t, "Default v2", reverted.Content, "revert re-syncs the repository version")

	_, err = f.svc.Revert(ctx, id)
	assert.ErrorIs(t, err, promptservice.ErrPromptNotOverride)
}

func TestOverrideAndRevertAuthorizeTheCredentialWorkspace(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["hook.md"] = "Default"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	denied := errors.New("denied")
	f.svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return denied })
	_, err = f.svc.Override(ctx, syncedPromptID(t, f, "hook"))
	assert.ErrorIs(t, err, denied)
}

func TestOverrideRoutes(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["hook.md"] = "Default"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	id := syncedPromptID(t, f, "hook")

	member := newTestRouter(t, f.svc, authn.Identity{UserID: "m", Role: authn.RoleMember, OrgID: "org-1"})
	assert.Equal(t, http.StatusForbidden, doRequest(member, http.MethodPost, "/api/v1/prompt-sync/prompts/"+id+"/override", "").Code)

	router := newTestRouter(t, f.svc, admin)
	overridden := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/prompts/"+id+"/override", "")
	require.Equal(t, http.StatusOK, overridden.Code, overridden.Body.String())
	var body struct {
		Source string `json:"source"`
	}
	require.NoError(t, json.Unmarshal(overridden.Body.Bytes(), &body))
	assert.Equal(t, "override", body.Source)

	again := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/prompts/"+id+"/override", "")
	assert.Equal(t, http.StatusConflict, again.Code)
	assert.Equal(t, http.StatusNotFound, doRequest(router, http.MethodPost, "/api/v1/prompt-sync/prompts/missing/revert", "").Code)

	reverted := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/prompts/"+id+"/revert", "")
	require.Equal(t, http.StatusOK, reverted.Code, reverted.Body.String())
}
