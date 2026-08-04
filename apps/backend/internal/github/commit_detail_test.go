package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestPRCommitStatsAreMarkedUnavailable(t *testing.T) {
	commits, err := parsePRCommitsJSON(`[
		{
			"sha": "1111111111111111111111111111111111111111",
			"commit": {
				"message": "Remote commit",
				"author": {"date": "2026-08-04T10:00:00Z"}
			},
			"author": {"login": "octocat"}
		}
	]`)
	if err != nil {
		t.Fatalf("parse PR commits: %v", err)
	}

	encoded, err := json.Marshal(commits)
	if err != nil {
		t.Fatalf("marshal PR commits: %v", err)
	}
	if !strings.Contains(string(encoded), `"stats_available":false`) {
		t.Fatalf("PR commit stats availability = %s, want explicit false", encoded)
	}
}

func TestParsePRCommitDetailJSONMergesPages(t *testing.T) {
	detail, err := parsePRCommitDetailJSON(`[
		{
			"sha": "2222222222222222222222222222222222222222",
			"commit": {
				"message": "Remote detail\nbody",
				"author": {"name": "Octo Cat", "date": "2026-08-04T11:00:00Z"}
			},
			"author": {"login": "octocat"},
			"stats": {"additions": 7, "deletions": 3, "total": 10},
			"files": [
				{"filename": "one.txt", "status": "modified", "additions": 4, "deletions": 1, "patch": "@@ -1 +1 @@\n-old\n+new"},
				{"filename": "duplicate.txt", "status": "added", "additions": 1, "deletions": 0}
			]
		},
		{
			"sha": "2222222222222222222222222222222222222222",
			"commit": {"message": "ignored page metadata", "author": {"name": "Other", "date": "2026-08-05T11:00:00Z"}},
			"author": {"login": "other"},
			"stats": {"additions": 99, "deletions": 99, "total": 198},
			"files": [
				{"filename": "duplicate.txt", "status": "added", "additions": 1, "deletions": 0},
				{"filename": "two.txt", "status": "removed", "additions": 0, "deletions": 2, "patch": "@@ -1 +0,0 @@\n-gone"}
			]
		}
	]`)
	if err != nil {
		t.Fatalf("parse PR commit detail: %v", err)
	}
	if detail.SHA != "2222222222222222222222222222222222222222" || detail.Message != "Remote detail\nbody" {
		t.Fatalf("metadata = %#v", detail)
	}
	if detail.AuthorLogin != "octocat" || detail.AuthorName != "Octo Cat" || detail.AuthorDate != "2026-08-04T11:00:00Z" {
		t.Fatalf("author = %#v", detail)
	}
	if detail.Additions != 7 || detail.Deletions != 3 || detail.FilesChanged != 3 {
		t.Fatalf("stats = %#v", detail)
	}
	if len(detail.Files) != 3 {
		t.Fatalf("files = %#v, want three provider-ordered unique files", detail.Files)
	}
	if detail.Files[0].Filename != "one.txt" || detail.Files[1].Filename != "duplicate.txt" || detail.Files[2].Filename != "two.txt" {
		t.Fatalf("file order = %#v", detail.Files)
	}
	if !strings.Contains(detail.Files[0].Patch, "diff --git a/one.txt b/one.txt") {
		t.Fatalf("file patch = %q, want complete diff", detail.Files[0].Patch)
	}
}

func TestGetPRCommitDetailForWorkspaceUsesAuthorizedClient(t *testing.T) {
	const sha = "3333333333333333333333333333333333333333"
	client := NewMockClient()
	client.AddRepos("test-user", []GitHubRepo{{FullName: "acme/widget", Owner: "acme", Name: "widget"}})
	client.AddPRCommitDetail("acme", "widget", sha, PRCommitDetail{
		SHA:   sha,
		Files: []PRFile{{Filename: "remote.txt", Status: "modified", Patch: "remote patch"}},
	})
	svc := newWorkspaceAuthenticatedTestService(t, client, nil, "workspace-1")

	detail, err := svc.GetPRCommitDetailForWorkspace(context.Background(), "workspace-1", "", "acme", "widget", sha)
	if err != nil {
		t.Fatalf("GetPRCommitDetailForWorkspace: %v", err)
	}
	if detail.SHA != sha || len(detail.Files) != 1 || detail.Files[0].Filename != "remote.txt" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestGetPRCommitDetailForWorkspaceRejectsUnsafeSHA(t *testing.T) {
	svc := &Service{workspaceAuthorizer: func(context.Context, string) error {
		return errors.New("workspace lookup should not run")
	}}

	_, err := svc.GetPRCommitDetailForWorkspace(context.Background(), "workspace-1", "", "acme", "widget", "main")
	if err == nil || !strings.Contains(err.Error(), "invalid commit SHA") {
		t.Fatalf("error = %v, want invalid commit SHA before provider access", err)
	}
}

func TestWSGetPRCommitDetailRoutesWorkspaceRequest(t *testing.T) {
	const sha = "4444444444444444444444444444444444444444"
	client := NewMockClient()
	client.AddRepos("test-user", []GitHubRepo{{FullName: "acme/widget", Owner: "acme", Name: "widget"}})
	client.AddPRCommitDetail("acme", "widget", sha, PRCommitDetail{SHA: sha, Message: "remote commit"})
	svc := newWorkspaceAuthenticatedTestService(t, client, nil, "workspace-1")
	message, err := ws.NewRequest("request-1", "github.pr_commit.get", map[string]any{
		"workspace_id": "workspace-1", "owner": "acme", "repo": "widget", "sha": sha,
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	response, err := wsGetPRCommitDetail(svc, testLogger(t))(context.Background(), message)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if response.Type != ws.MessageTypeResponse {
		t.Fatalf("response type = %q, want response", response.Type)
	}
	var body struct {
		Commit PRCommitDetail `json:"commit"`
	}
	if err := json.Unmarshal(response.Payload, &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Commit.SHA != sha || body.Commit.Message != "remote commit" {
		t.Fatalf("response body = %#v", body)
	}
}

func TestWSGetPRCommitDetailReturnsWorkspaceAuthorizationError(t *testing.T) {
	svc := NewService(NewMockClient(), AuthMethodPAT, nil, nil, nil, testLogger(t))
	denied := errors.New("workspace not found")
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID != "workspace-1" {
			t.Fatalf("workspaceID = %q", workspaceID)
		}
		return denied
	})
	message, err := ws.NewRequest("request-1", "github.pr_commit.get", map[string]any{
		"workspace_id": "workspace-1", "owner": "acme", "repo": "widget",
		"sha": "5555555555555555555555555555555555555555",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	response, err := wsGetPRCommitDetail(svc, testLogger(t))(context.Background(), message)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if response.Type != ws.MessageTypeError || !strings.Contains(string(response.Payload), denied.Error()) {
		t.Fatalf("response = %#v, want workspace authorization error", response)
	}
}
