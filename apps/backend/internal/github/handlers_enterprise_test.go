package github

import (
	"context"
	"strings"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestEnterpriseWebSocketRejectsStalePRHost(t *testing.T) {
	for _, action := range []string{
		ws.ActionGitHubPRFeedbackGet,
		ws.ActionGitHubPRFilesGet,
		ws.ActionGitHubPRCommitsGet,
		ws.ActionGitHubPRCommitGet,
	} {
		t.Run(action, func(t *testing.T) {
			svc := NewService(nil, AuthMethodNone, nil, newTestStore(t), nil, testLogger(t))
			configureTestWorkspaceAuth(t, svc, NewMockClient(), "workspace")
			authorized := false
			svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
				authorized = true
				return nil
			})
			dispatcher := ws.NewDispatcher()
			registerWSHandlers(dispatcher, svc, testLogger(t))
			request, err := ws.NewRequest("stale-host", action, map[string]any{
				"workspace_id": "workspace", "owner": "acme", "repo": "widget",
				"number": 7, "sha": strings.Repeat("a", 40), "expected_host": "git.example.com",
			})
			if err != nil {
				t.Fatal(err)
			}
			response, err := dispatcher.Dispatch(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if response.Type != ws.MessageTypeError || !strings.Contains(string(response.Payload), ErrGitHubHostMismatch.Error()) {
				t.Fatalf("response = %s; want host mismatch", response.Payload)
			}
			if !authorized {
				t.Fatal("host guard bypassed workspace authorization")
			}
		})
	}
}
