package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.15
func TestRegisteredWorkspaceUpdate(t *testing.T) {
	for _, transport := range []string{"http", "websocket"} {
		t.Run(transport, func(t *testing.T) {
			store := newTestSQLiteStore(t)
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			require.NoError(t, err)
			svc := NewService(NewUserVisibleStore(store), log)
			allowed := true
			svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
				if !allowed || workspaceID != "workspace-a" {
					return ErrWorkspaceAccessDenied
				}
				return nil
			})
			owner := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
			foreign := authn.WithIdentity(context.Background(), authn.Identity{UserID: "foreign", Role: authn.RoleMember})
			created, err := svc.Create(owner, &CreateSecretRequest{
				Name: "original", Value: "synthetic-original", Scope: ScopeWorkspace, WorkspaceID: "workspace-a",
			})
			require.NoError(t, err)
			router := gin.New()
			dispatcher := ws.NewDispatcher()
			RegisterRoutes(router, dispatcher, svc, log)
			update := registeredUpdate(t, transport, router, dispatcher, created.ID)
			// The first case is rejected by the store's user_id ownership filter
			// (the secret was created with "owner", so "foreign" does not match),
			// not by the authorizer mock, which allows workspace-a while allowed is
			// true. The authorizer covers the remaining three cases.
			for _, denied := range []struct {
				ctx       context.Context
				workspace string
				allowed   bool
			}{
				{foreign, "workspace-a", true}, {owner, "workspace-b", true}, {owner, "", true},
				{owner, "workspace-a", false},
			} {
				allowed = denied.allowed
				_, ok := update(denied.ctx, denied.workspace)
				allowed = true
				require.False(t, ok)
				metadata, err := svc.GetWorkspaceSecret(owner, created.ID, "workspace-a")
				require.NoError(t, err)
				require.Equal(t, "original", metadata.Name)
				value, err := svc.RevealWorkspaceSecret(owner, created.ID, "workspace-a")
				require.NoError(t, err)
				require.True(t, value == "synthetic-original", "denied update mutated stored value")
			}
			payload, ok := update(owner, "workspace-a")
			require.True(t, ok)
			require.NotContains(t, string(payload), "synthetic-rotated")
			var item SecretListItem
			require.NoError(t, json.Unmarshal(payload, &item))
			require.Equal(t, created.ID, item.ID)
			require.Equal(t, "renamed", item.Name)
			require.Equal(t, ScopeWorkspace, item.Scope)
			require.Equal(t, "workspace-a", item.WorkspaceID)
			require.True(t, item.HasValue)
			require.Equal(t, created.CreatedAt, item.CreatedAt)
			require.False(t, item.UpdatedAt.Before(created.UpdatedAt))
			assertStoredRotation(t, store, owner, created.ID)
		})
	}
}

func registeredUpdate(t *testing.T, transport string, router *gin.Engine, dispatcher *ws.Dispatcher, id string) func(context.Context, string) ([]byte, bool) {
	t.Helper()
	return func(ctx context.Context, workspaceID string) ([]byte, bool) {
		if transport == "http" {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/"+id+"?workspace_id="+workspaceID,
				strings.NewReader(`{"name":"renamed","value":"synthetic-rotated"}`)).WithContext(ctx)
			router.ServeHTTP(rec, req)
			return rec.Body.Bytes(), rec.Code == http.StatusOK
		}
		payload, err := json.Marshal(map[string]string{
			"id": id, "workspace_id": workspaceID, "name": "renamed", "value": "synthetic-rotated",
		})
		require.NoError(t, err)
		response, err := dispatcher.Dispatch(ctx, &ws.Message{
			ID: "update-request", Type: ws.MessageTypeRequest, Action: ws.ActionSecretUpdate, Payload: payload,
		})
		require.NoError(t, err)
		require.NotNil(t, response)
		return response.Payload, response.Type == ws.MessageTypeResponse
	}
}

func assertStoredRotation(t *testing.T, store *sqliteStore, ctx context.Context, id string) {
	t.Helper()
	var ciphertext, nonce []byte
	require.NoError(t, store.db.QueryRowContext(ctx,
		"SELECT encrypted_value, nonce FROM secrets WHERE id = ?", id).Scan(&ciphertext, &nonce))
	require.NotContains(t, string(ciphertext), "synthetic-rotated")
	require.Len(t, nonce, 12)
	// Construct a fresh store over the committed database and reload the on-disk key.
	key, err := NewMasterKeyProvider(filepath.Dir(store.crypto.keyPath))
	require.NoError(t, err)
	reloaded, closeStore, err := Provide(store.db, store.ro, key)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeStore()) })
	value, err := NewUserVisibleStore(reloaded).(ScopedSecretStore).RevealForWorkspace(ctx, id, "workspace-a")
	require.NoError(t, err)
	require.True(t, value == "synthetic-rotated", "stored rotation did not decrypt")
}
