package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func seedRemoteURLSettingsRepository(t *testing.T, f *checkoutSettingsFixture) {
	t.Helper()
	row, err := f.store.GetRepository(t.Context(), "settings-repo")
	require.NoError(t, err)
	row.Name = "acme/widget"
	row.SourceType = "provider"
	row.Provider = "github"
	row.ProviderHost = "https://github.com"
	row.ProviderOwner = "acme"
	row.ProviderName = "widget"
	row.RemoteURL = "git@github.com:acme/widget.git"
	require.NoError(t, f.store.UpdateRepository(t.Context(), row))
}

func (f *checkoutSettingsFixture) saveRemoteURL(
	t *testing.T, ctx context.Context, changes map[string]any, options map[string]any,
) *ws.Message {
	t.Helper()
	payload := map[string]any{
		"target":  map[string]any{"resource_type": "repository", "resource_id": "settings-repo", "workspace_id": "settings-ws"},
		"changes": changes,
	}
	if options != nil {
		payload["options"] = options
	}
	request, err := ws.NewRequest("remote-url-settings", ws.ActionMCPUpdateSettings, payload)
	require.NoError(t, err)
	response, err := f.dispatch.Dispatch(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
}

func TestRepositoryRemoteURLGuardedMCP(t *testing.T) {
	t.Run("preview_is_write_free_then_update_readback", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		seedRemoteURLSettingsRepository(t, f)
		ctx := checkoutSettingsContext(t.Context())

		response := f.saveRemoteURL(t, ctx, map[string]any{"remote_url": "https://github.com/acme/widget.git"}, map[string]any{"validate_only": true})
		require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)
		var preview struct {
			Validation taskservice.RepositoryRemoteURLPreview `json:"validation"`
			Validated  bool                                   `json:"validated"`
		}
		require.NoError(t, json.Unmarshal(response.Payload, &preview))
		require.True(t, preview.Validated)
		require.Equal(t, "https://github.com/acme/widget.git", preview.Validation.EffectiveRemoteURL)
		require.Equal(t, "locator_identity", preview.Validation.ValidationScope)
		require.Empty(t, f.events.rows)
		stored, err := f.store.GetRepository(ctx, "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)

		response = f.saveRemoteURL(t, ctx, map[string]any{"remote_url": "https://github.com/acme/widget.git"}, nil)
		require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)
		var payload struct {
			Settings struct {
				RemoteURL string `json:"remote_url"`
			} `json:"settings"`
		}
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, "https://github.com/acme/widget.git", payload.Settings.RemoteURL)
		stored, err = f.store.GetRepository(ctx, "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "https://github.com/acme/widget.git", stored.RemoteURL)
		require.Len(t, f.events.rows, 1)
		require.Equal(t, "https://github.com/acme/widget.git", f.events.rows[0].Data.(map[string]interface{})["remote_url"])
	})

	for _, test := range []struct {
		name    string
		changes map[string]any
		options map[string]any
		code    string
	}{
		{name: "invalid_url", changes: map[string]any{"remote_url": "https://token@github.com/acme/widget.git"}, options: map[string]any{"validate_only": true}, code: "repository_selection_invalid"},
		{name: "wrong_identity", changes: map[string]any{"remote_url": "https://github.com/other/widget.git"}, options: map[string]any{"validate_only": true}, code: "repository_selection_invalid"},
		{name: "non_boolean_option", changes: map[string]any{"remote_url": "https://github.com/acme/widget.git"}, options: map[string]any{"validate_only": "yes"}, code: "invalid_option"},
		{name: "null_option", changes: map[string]any{"remote_url": "https://github.com/acme/widget.git"}, options: map[string]any{"validate_only": nil}, code: "invalid_option"},
		{name: "unknown_option", changes: map[string]any{"remote_url": "https://github.com/acme/widget.git"}, options: map[string]any{"dry_run": true}, code: "unknown_option"},
		{name: "mixed_preview_changes", changes: map[string]any{"remote_url": "https://github.com/acme/widget.git", "name": "renamed"}, options: map[string]any{"validate_only": true}, code: "validate_only_requires_remote_url"},
		{name: "null_url", changes: map[string]any{"remote_url": nil}, options: map[string]any{"validate_only": true}, code: "invalid_value_type"},
		{name: "blank_url", changes: map[string]any{"remote_url": "   "}, options: map[string]any{"validate_only": true}, code: "repository_selection_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := checkoutSettingsHarness(t)
			seedRemoteURLSettingsRepository(t, f)
			before, err := f.store.GetRepository(t.Context(), "settings-repo")
			require.NoError(t, err)
			response := f.saveRemoteURL(t, checkoutSettingsContext(t.Context()), test.changes, test.options)
			require.Equal(t, ws.MessageTypeError, response.Type, "%s", response.Payload)
			var payload ws.ErrorPayload
			require.NoError(t, json.Unmarshal(response.Payload, &payload))
			require.Equal(t, ws.ErrorCodeValidation, payload.Code)
			require.Equal(t, test.code, payload.Details["code"])
			stored, err := f.store.GetRepository(t.Context(), "settings-repo")
			require.NoError(t, err)
			require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
			require.Equal(t, before.UpdatedAt, stored.UpdatedAt)
			require.Len(t, stored.SecretBindings, len(before.SecretBindings))
			require.Empty(t, f.events.rows)
		})
	}

	t.Run("read_only_workspace_is_refused", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		seedRemoteURLSettingsRepository(t, f)
		workspace, err := f.store.GetWorkspace(t.Context(), "settings-ws")
		require.NoError(t, err)
		workspace.Name = "Improve Kandev"
		require.NoError(t, f.store.UpdateWorkspace(t.Context(), workspace))

		response := f.saveRemoteURL(t, checkoutSettingsContext(t.Context()), map[string]any{"remote_url": "https://github.com/acme/widget.git"}, map[string]any{"validate_only": true})
		require.Equal(t, ws.MessageTypeError, response.Type)
		var payload ws.ErrorPayload
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, "repository_read_only", payload.Details["code"])
		stored, err := f.store.GetRepository(t.Context(), "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
	})

	t.Run("validate_only_false_still_validates", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		seedRemoteURLSettingsRepository(t, f)
		response := f.saveRemoteURL(t, checkoutSettingsContext(t.Context()), map[string]any{"remote_url": "https://github.com/acme/widget.git"}, map[string]any{"validate_only": false})
		require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)
		stored, err := f.store.GetRepository(t.Context(), "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "https://github.com/acme/widget.git", stored.RemoteURL)
	})

	t.Run("null_option_leaves_bindings_untouched", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		seedRemoteURLSettingsRepository(t, f)
		require.NoError(t, f.store.ReplaceRepositorySecretBindings(t.Context(), "settings-repo", []models.RepositorySecretBinding{
			{Key: "TOKEN", SecretID: "secret-ref"},
		}))
		before, err := f.store.GetRepository(t.Context(), "settings-repo")
		require.NoError(t, err)
		require.Len(t, before.SecretBindings, 1)
		response := f.saveRemoteURL(t, checkoutSettingsContext(t.Context()),
			map[string]any{"remote_url": "https://github.com/acme/widget.git"},
			map[string]any{"validate_only": nil})
		require.Equal(t, ws.MessageTypeError, response.Type, "%s", response.Payload)
		var payload ws.ErrorPayload
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, "invalid_option", payload.Details["code"])
		stored, err := f.store.GetRepository(t.Context(), "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
		require.Equal(t, before.UpdatedAt, stored.UpdatedAt)
		require.Len(t, stored.SecretBindings, 1)
		require.Equal(t, "TOKEN", stored.SecretBindings[0].Key)
		require.Empty(t, f.events.rows)
	})

	t.Run("validate_only_false_allows_an_ordinary_change", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		seedRemoteURLSettingsRepository(t, f)
		response := f.saveRemoteURL(t, checkoutSettingsContext(t.Context()), map[string]any{"name": "renamed"}, map[string]any{"validate_only": false})
		require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)
		stored, err := f.store.GetRepository(t.Context(), "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "renamed", stored.Name)
		require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
		require.Len(t, f.events.rows, 1)
	})

	t.Run("validate_only_false_allows_a_combined_repair", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		seedRemoteURLSettingsRepository(t, f)
		response := f.saveRemoteURL(t, checkoutSettingsContext(t.Context()),
			map[string]any{"remote_url": "https://github.com/acme/widget.git", "name": "renamed"},
			map[string]any{"validate_only": false})
		require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)
		stored, err := f.store.GetRepository(t.Context(), "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "renamed", stored.Name)
		require.Equal(t, "https://github.com/acme/widget.git", stored.RemoteURL)
		require.Len(t, f.events.rows, 1)
		require.Equal(t, "https://github.com/acme/widget.git", f.events.rows[0].Data.(map[string]interface{})["remote_url"])
	})

	t.Run("validate_only_false_allows_a_combined_binding_repair", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		seedRemoteURLSettingsRepository(t, f)
		key, err := secrets.NewMasterKeyProvider(t.TempDir())
		require.NoError(t, err)
		catalog, closeCatalog, err := secrets.Provide(f.db, f.db, key)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, closeCatalog()) })
		secret := &secrets.SecretWithValue{Secret: secrets.Secret{Name: "Repair token", Scope: secrets.ScopeWorkspace, WorkspaceID: "settings-ws"}, Value: "disposable-sensitive-value"}
		require.NoError(t, catalog.Create(t.Context(), secret))
		f.api.SetSecretStore(catalog)

		response := f.saveRemoteURL(t, checkoutSettingsContext(t.Context()),
			map[string]any{
				"remote_url":      "https://github.com/acme/widget.git",
				"secret_bindings": []any{map[string]any{"key": "TOKEN", "secret_id": secret.ID}},
			},
			map[string]any{"validate_only": false})
		require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)
		stored, err := f.store.GetRepository(t.Context(), "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "https://github.com/acme/widget.git", stored.RemoteURL)
		require.Len(t, stored.SecretBindings, 1)
		require.Equal(t, "TOKEN", stored.SecretBindings[0].Key)
		require.Len(t, f.events.rows, 1)
	})
}

func TestRepositoryValidateOnlyOptionRejectsNonBooleanJSONValues(t *testing.T) {
	for _, raw := range []string{"null", `"true"`, "1", "{}"} {
		_, err := repositoryValidateOnlyOption(map[string]json.RawMessage{"validate_only": json.RawMessage(raw)})
		require.Error(t, err, raw)
	}
	_, err := repositoryValidateOnlyOption(map[string]json.RawMessage{"dry_run": json.RawMessage("true")})
	require.Error(t, err)
}
