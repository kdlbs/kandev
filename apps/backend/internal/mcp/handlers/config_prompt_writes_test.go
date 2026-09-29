package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func newPromptWriteFixture(t *testing.T) (*Handlers, *promptservice.Service, *ws.Dispatcher) {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "prompts.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, _, err := promptstore.Provide(database, database)
	require.NoError(t, err)
	svc := promptservice.NewService(repo)
	h := &Handlers{logger: testLogger(t)}
	h.SetPromptReader(svc)
	h.SetPromptWriter(svc, func() bool { return true })
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	return h, svc, dispatcher
}

func TestSharedPromptWriteAuthorizationAndReadback(t *testing.T) {
	h, svc, dispatcher := newPromptWriteFixture(t)
	for _, tc := range []struct {
		name              string
		identity          *authn.Identity
		disabled, allowed bool
	}{
		{name: "missing"},
		{name: "disabled", disabled: true, allowed: true},
		{name: "synthetic", identity: &authn.Identity{Synthetic: true}, allowed: true},
		{name: "admin", identity: &authn.Identity{UserID: "admin", Role: authn.RoleAdmin}, allowed: true},
		{name: "member", identity: &authn.Identity{UserID: "member", Role: authn.RoleMember}},
		{name: "guest", identity: &authn.Identity{UserID: "guest", Role: authn.RoleGuest}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.identity != nil {
				ctx = authn.WithIdentity(ctx, *tc.identity)
			}
			h.promptAuthEnabled = func() bool { return !tc.disabled }
			resp, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPCreateSharedPrompt, map[string]any{"name": tc.name, "content": " Created content "}))
			require.NoError(t, err)
			if !tc.allowed {
				assertWSError(t, resp, ws.ErrorCodeForbidden)
				_, err = svc.GetPromptByName(context.Background(), tc.name)
				require.ErrorIs(t, err, promptservice.ErrPromptNotFound)
				return
			}
			require.Equal(t, ws.MessageTypeResponse, resp.Type)
			resp, err = dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPUpdateSharedPrompt, map[string]any{"name": tc.name, "content": " Revised é "}))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type)
			var result sharedPromptRead
			require.NoError(t, json.Unmarshal(resp.Payload, &result))
			require.True(t, result.AllowAgentEdits)
			require.Equal(t, "Revised é", result.Content)
			require.Equal(t, len("Revised é"), result.ContentBytes)
			saved, err := svc.GetPromptByName(ctx, tc.name)
			require.NoError(t, err)
			require.Equal(t, result.Content, saved.Content)
		})
	}
}

type failedPromptWriter struct {
	result *promptmodels.Prompt
	err    error
}

func (s failedPromptWriter) CreatePromptForAgent(context.Context, string, string) (*promptmodels.Prompt, error) {
	return s.result, s.err
}
func (s failedPromptWriter) UpdatePromptContentForAgent(context.Context, string, string) (*promptmodels.Prompt, error) {
	return s.result, s.err
}

func TestSharedPromptWriteErrors(t *testing.T) {
	h, svc, dispatcher := newPromptWriteFixture(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true})
	human, err := svc.CreatePrompt(ctx, "human", "Original")
	require.NoError(t, err)
	for _, tc := range []struct{ action, name, content, code string }{
		{ws.ActionMCPCreateSharedPrompt, "human", "Duplicate", ws.ErrorCodeValidation},
		{ws.ActionMCPUpdateSharedPrompt, "code-review", "Overwrite builtin", ws.ErrorCodeForbidden},
		{ws.ActionMCPUpdateSharedPrompt, "human", "Overwrite human", ws.ErrorCodeForbidden},
		{ws.ActionMCPUpdateSharedPrompt, "missing", "Missing", ws.ErrorCodeNotFound},
		{ws.ActionMCPCreateSharedPrompt, " ", "Content", ws.ErrorCodeValidation},
		{ws.ActionMCPCreateSharedPrompt, "empty-content", " ", ws.ErrorCodeValidation},
	} {
		resp, err := dispatcher.Dispatch(ctx, makeWSMessage(t, tc.action, map[string]any{"name": tc.name, "content": tc.content, "allow_agent_edits": true}))
		require.NoError(t, err)
		assertWSError(t, resp, tc.code)
	}
	saved, err := svc.GetPromptByName(ctx, human.Name)
	require.NoError(t, err)
	require.Equal(t, human.Content, saved.Content)
	require.False(t, saved.AllowAgentEdits)
	for _, writer := range []failedPromptWriter{
		{result: &promptmodels.Prompt{Content: "secret"}, err: errors.New("private SQL detail")}, {},
	} {
		h.SetPromptWriter(writer, nil)
		resp, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPCreateSharedPrompt, map[string]any{"name": "safe", "content": "content"}))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeInternalError)
		require.NotContains(t, string(resp.Payload), "private SQL detail")
		require.NotContains(t, string(resp.Payload), "secret")
	}
}
