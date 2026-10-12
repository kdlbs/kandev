package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestContributionHistoryHandler(t *testing.T) {
	h, server := gitHandlerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/git/contribution/history-explanation" {
			t.Errorf("path = %s, want history explanation endpoint", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["branch"] != "feature/history" || body["repo"] != "svc" {
			t.Errorf("body = %#v, want branch and repo", body)
		}
		_, _ = w.Write([]byte(`{
			"repo":"svc",
			"branch":"feature/history",
			"expected_local_head":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"expected_remote_head":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"kind":"unexplained",
			"reason":"no_matching_reflog"
		}`))
	})
	defer server.Close()

	msg, err := ws.NewRequest("id", ws.ActionWorktreeContributionHistoryExplanation, GitContributionHistoryExplanationRequest{
		SessionID:          "s",
		Repo:               "svc",
		Branch:             "feature/history",
		ExpectedLocalHead:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpectedRemoteHead: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := h.wsContributionHistoryExplanation(context.Background(), msg)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if response == nil || !json.Valid(response.Payload) {
		t.Fatalf("response = %#v, want JSON payload", response)
	}
}

func TestContributionHistoryHandlerRejectsMissingIdentity(t *testing.T) {
	h := NewGitHandlers(nil, nil, newTestLogger())
	msg, err := ws.NewRequest("id", ws.ActionWorktreeContributionHistoryExplanation, GitContributionHistoryExplanationRequest{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.wsContributionHistoryExplanation(context.Background(), msg); err == nil {
		t.Fatal("expected missing identity validation error")
	}
}
