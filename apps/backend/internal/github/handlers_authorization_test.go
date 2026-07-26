package github

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestReviewWatchWebSocketDeniesForeignWorkspaceListAndMutations(t *testing.T) {
	store := newTestStore(t)
	watch := &ReviewWatch{WorkspaceID: "workspace-b", Prompt: "workspace-b review secret"}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("create victim review watch: %v", err)
	}

	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	svc.SetWorkspaceAuthorizer(func(ctx context.Context, workspaceID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.UserID != "member-a" || identity.Role != authn.RoleMember {
			t.Fatalf("authorizer identity = %#v, %v; want authenticated member-a", identity, ok)
		}
		if workspaceID != watch.WorkspaceID {
			t.Fatalf("authorizer workspace = %q, want %q", workspaceID, watch.WorkspaceID)
		}
		return repoerrors.ErrWorkspaceNotFound
	})

	dispatcher := ws.NewDispatcher()
	registerWSHandlers(dispatcher, svc, logger.Default())
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-a", Role: authn.RoleMember})

	for _, tc := range []struct {
		name    string
		action  string
		payload map[string]any
	}{
		{
			name:    "list",
			action:  ws.ActionGitHubReviewWatchesList,
			payload: map[string]any{"workspace_id": watch.WorkspaceID},
		},
		{
			name:    "update",
			action:  ws.ActionGitHubReviewWatchUpdate,
			payload: map[string]any{"id": watch.ID, "prompt": "attacker update"},
		},
		{
			name:    "delete",
			action:  ws.ActionGitHubReviewWatchDelete,
			payload: map[string]any{"id": watch.ID},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := ws.NewRequest("request-"+tc.name, tc.action, tc.payload)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			response, err := dispatcher.Dispatch(ctx, request)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if response.Type != ws.MessageTypeError || !strings.Contains(string(response.Payload), ws.ErrorCodeNotFound) {
				t.Fatalf("response = %#v, want sanitized NOT_FOUND", response)
			}
			if strings.Contains(string(response.Payload), watch.ID) || strings.Contains(string(response.Payload), watch.Prompt) {
				t.Fatalf("response = %s, must not disclose victim watch metadata", response.Payload)
			}
		})
	}

	remaining, err := store.GetReviewWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("load victim review watch after denied mutations: %v", err)
	}
	if remaining == nil || remaining.Prompt != watch.Prompt {
		t.Fatalf("victim review watch = %#v, want unchanged prompt %q", remaining, watch.Prompt)
	}
}
