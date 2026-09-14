package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestContributionHistoryClient(t *testing.T) {
	localHead := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	remoteHead := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"repo":"svc",
		"branch":"feature/history",
		"expected_local_head":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"expected_remote_head":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"kind":"local_rebase",
		"reason":"matched_reflog",
		"onto_head":"cccccccccccccccccccccccccccccccccccccccc",
		"task_commit_count":5,
		"published_commit_count":5,
		"new_base_commit_count":29
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GitContributionHistoryExplanation(
		context.Background(), "feature/history", localHead, remoteHead, "svc")
	if err != nil {
		t.Fatalf("GitContributionHistoryExplanation: %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/git/contribution/history-explanation" {
		t.Fatalf("request = %s %s, want POST /api/v1/git/contribution/history-explanation", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got.ContentType)
	}
	var body map[string]any
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	for key, want := range map[string]any{
		"repo":                 "svc",
		"branch":               "feature/history",
		"expected_local_head":  localHead,
		"expected_remote_head": remoteHead,
	} {
		if body[key] != want {
			t.Errorf("body[%q] = %#v, want %#v", key, body[key], want)
		}
	}
	if result.Kind != "local_rebase" || result.Reason != "matched_reflog" ||
		result.TaskCommitCount == nil || *result.TaskCommitCount != 5 {
		t.Fatalf("result = %+v, want decoded explanation", result)
	}
}
