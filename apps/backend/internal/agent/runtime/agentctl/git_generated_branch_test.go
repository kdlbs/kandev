package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestGitRenameGeneratedBranchSendsExpectedBranch(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, gitOperationOK))
	result, err := newHTTPOnlyClient(srv.URL).GitRenameGeneratedBranch(
		context.Background(), "feature/provisional", "feature/final", "svc")
	if err != nil {
		t.Fatalf("GitRenameGeneratedBranch() error = %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/git/rename-branch" {
		t.Fatalf("request = %s %s, want POST rename route", got.Method, got.Path)
	}
	var body map[string]any
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	want := map[string]any{
		"new_name": "feature/final", "repo": "svc", "expected_generated_branch": "feature/provisional",
	}
	if len(body) != len(want) {
		t.Fatalf("request body = %s, want exactly %v", got.Body, want)
	}
	for key, value := range want {
		if body[key] != value {
			t.Errorf("body[%q] = %#v, want %#v", key, body[key], value)
		}
	}
	if !result.Success {
		t.Fatalf("result = %+v, want success", result)
	}
}

func TestGitRenameBranchOmitsGeneratedIntent(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, gitOperationOK))
	if _, err := newHTTPOnlyClient(srv.URL).GitRenameBranch(context.Background(), "feature/final", ""); err != nil {
		t.Fatalf("GitRenameBranch() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if _, present := body["expected_generated_branch"]; present {
		t.Fatalf("generic rename body = %s, want no generated intent", got.Body)
	}
}
