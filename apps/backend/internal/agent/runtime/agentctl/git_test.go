package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// gitOperationOK is the canonical success body every /api/v1/git/* POST returns.
const gitOperationOK = `{"success":true,"operation":"pull","output":"Already up to date."}`

// TestGitOperations_PostExpectedPathAndPayload pins the endpoint and JSON body
// of every thin wrapper over gitOperation. A wrapper that posts to the wrong
// path or drops a field is the whole failure mode for this layer.
func TestGitOperations_PostExpectedPathAndPayload(t *testing.T) {
	tests := []struct {
		name     string
		call     func(c *Client) (*GitOperationResult, error)
		wantPath string
		wantBody map[string]any
	}{
		{
			name:     "pull with rebase",
			call:     func(c *Client) (*GitOperationResult, error) { return c.GitPull(context.Background(), true, "svc") },
			wantPath: "/api/v1/git/pull",
			wantBody: map[string]any{"rebase": true, "repo": "svc"},
		},
		{
			name:     "pull without rebase omits empty repo",
			call:     func(c *Client) (*GitOperationResult, error) { return c.GitPull(context.Background(), false, "") },
			wantPath: "/api/v1/git/pull",
			wantBody: map[string]any{"rebase": false},
		},
		{
			name: "push force with upstream",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitPush(context.Background(), true, true, "svc")
			},
			wantPath: "/api/v1/git/push",
			wantBody: map[string]any{"force": true, "set_upstream": true, "repo": "svc"},
		},
		{
			name: "push plain",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitPush(context.Background(), false, false, "")
			},
			wantPath: "/api/v1/git/push",
			wantBody: map[string]any{"force": false, "set_upstream": false},
		},
		{
			name:     "push preflight",
			call:     func(c *Client) (*GitOperationResult, error) { return c.GitPushPreflight(context.Background(), "svc") },
			wantPath: "/api/v1/git/push-preflight",
			wantBody: map[string]any{"repo": "svc"},
		},
		{
			name: "replace remote contribution",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitReplaceRemoteContribution(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "svc")
			},
			wantPath: "/api/v1/git/contribution/replace",
			wantBody: map[string]any{"expected_remote_head": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "repo": "svc"},
		},
		{
			name: "use remote contribution",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitUseRemoteContribution(context.Background(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "svc")
			},
			wantPath: "/api/v1/git/contribution/use",
			wantBody: map[string]any{"expected_remote_head": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "repo": "svc"},
		},
		{
			name: "rebase onto base branch",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitRebase(context.Background(), "main", "svc")
			},
			wantPath: "/api/v1/git/rebase",
			wantBody: map[string]any{"base_branch": "main", "repo": "svc"},
		},
		{
			name: "merge base branch",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitMerge(context.Background(), "develop", "")
			},
			wantPath: "/api/v1/git/merge",
			wantBody: map[string]any{"base_branch": "develop"},
		},
		{
			name: "abort rebase",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitAbort(context.Background(), "rebase", "svc")
			},
			wantPath: "/api/v1/git/abort",
			wantBody: map[string]any{"operation": "rebase", "repo": "svc"},
		},
		{
			name: "commit with stage-all and amend",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitCommit(context.Background(), "fix: thing", true, true, "svc")
			},
			wantPath: "/api/v1/git/commit",
			wantBody: map[string]any{"message": "fix: thing", "stage_all": true, "amend": true, "repo": "svc"},
		},
		{
			name: "commit plain",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitCommit(context.Background(), "msg", false, false, "")
			},
			wantPath: "/api/v1/git/commit",
			wantBody: map[string]any{"message": "msg", "stage_all": false, "amend": false},
		},
		{
			name: "rename branch",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitRenameBranch(context.Background(), "feature/new", "svc")
			},
			wantPath: "/api/v1/git/rename-branch",
			wantBody: map[string]any{"new_name": "feature/new", "repo": "svc"},
		},
		{
			name: "stage explicit paths",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitStage(context.Background(), []string{"a.go", "b.go"}, "svc")
			},
			wantPath: "/api/v1/git/stage",
			wantBody: map[string]any{"paths": []any{"a.go", "b.go"}, "repo": "svc"},
		},
		{
			name: "stage all sends null paths",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitStage(context.Background(), nil, "")
			},
			wantPath: "/api/v1/git/stage",
			wantBody: map[string]any{"paths": nil},
		},
		{
			name: "unstage explicit paths",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitUnstage(context.Background(), []string{"a.go"}, "")
			},
			wantPath: "/api/v1/git/unstage",
			wantBody: map[string]any{"paths": []any{"a.go"}},
		},
		{
			name: "discard paths",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitDiscard(context.Background(), []string{"a.go"}, "svc")
			},
			wantPath: "/api/v1/git/discard",
			wantBody: map[string]any{"paths": []any{"a.go"}, "repo": "svc"},
		},
		{
			name: "revert commit",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitRevertCommit(context.Background(), "abc123", "svc")
			},
			wantPath: "/api/v1/git/revert-commit",
			wantBody: map[string]any{"commit_sha": "abc123", "repo": "svc"},
		},
		{
			name: "reset hard",
			call: func(c *Client) (*GitOperationResult, error) {
				return c.GitReset(context.Background(), "abc123", "hard", "svc")
			},
			wantPath: "/api/v1/git/reset",
			wantBody: map[string]any{"commit_sha": "abc123", "mode": "hard", "repo": "svc"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := captureServer(t, jsonResponder(http.StatusOK, gitOperationOK))

			result, err := tc.call(newHTTPOnlyClient(srv.URL))
			if err != nil {
				t.Fatalf("call: %v", err)
			}

			if got.Method != http.MethodPost {
				t.Errorf("method = %q, want POST", got.Method)
			}
			if got.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", got.Path, tc.wantPath)
			}
			if got.ContentType != "application/json" {
				t.Errorf("content-type = %q, want application/json", got.ContentType)
			}

			var sent map[string]any
			if err := json.Unmarshal(got.Body, &sent); err != nil {
				t.Fatalf("decode sent body %q: %v", got.Body, err)
			}
			if len(sent) != len(tc.wantBody) {
				t.Errorf("sent body has %d fields (%s), want %d (%v)",
					len(sent), got.Body, len(tc.wantBody), tc.wantBody)
			}
			for key, want := range tc.wantBody {
				gotVal, present := sent[key]
				if !present {
					t.Errorf("sent body missing %q: %s", key, got.Body)
					continue
				}
				if !jsonEqual(gotVal, want) {
					t.Errorf("sent %q = %#v, want %#v", key, gotVal, want)
				}
			}

			if !result.Success || result.Operation != "pull" || result.Output != "Already up to date." {
				t.Errorf("result = %+v, want the decoded success body", result)
			}
		})
	}
}

func TestGitUseRemoteContribution_DecodesRecoveryBranch(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,
		"operation":"use_remote_contribution",
		"output":"HEAD is now at provider",
		"recovery_branch":"kandev/recovery-123"
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GitUseRemoteContribution(
		context.Background(),
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"svc",
	)
	if err != nil {
		t.Fatalf("GitUseRemoteContribution: %v", err)
	}
	if result.RecoveryBranch != "kandev/recovery-123" {
		t.Errorf("RecoveryBranch = %q, want %q", result.RecoveryBranch, "kandev/recovery-123")
	}
}

func TestGitOperation_DecodesContributionErrorCode(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":false,
		"operation":"replace_remote_contribution",
		"error":"remote contribution head changed",
		"error_code":"lease_mismatch"
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GitReplaceRemoteContribution(
		context.Background(),
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"svc",
	)
	if err != nil {
		t.Fatalf("GitReplaceRemoteContribution: %v", err)
	}
	if result.ErrorCode != "lease_mismatch" {
		t.Errorf("ErrorCode = %q, want %q", result.ErrorCode, "lease_mismatch")
	}
}

// jsonEqual compares two values decoded from (or destined for) JSON.
func jsonEqual(a, b any) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(ab) == string(bb)
}

func TestGitOperation_DecodesConflictFilesOnHTTP409(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusConflict, `{
		"success":false,"operation":"merge","output":"CONFLICT",
		"error":"merge conflict","conflict_files":["a.go","dir/b.go"]
	}`))

	// 409 is the documented "operation in progress / conflicted" case: the
	// client must return the decoded result WITHOUT an error so callers can
	// read conflict_files.
	result, err := newHTTPOnlyClient(srv.URL).GitMerge(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("GitMerge on 409 returned error %v, want the result with no error", err)
	}
	if result.Success {
		t.Error("Success = true, want false")
	}
	if result.Operation != "merge" {
		t.Errorf("Operation = %q, want merge", result.Operation)
	}
	if result.Error != "merge conflict" {
		t.Errorf("Error = %q, want merge conflict", result.Error)
	}
	if len(result.ConflictFiles) != 2 ||
		result.ConflictFiles[0] != "a.go" || result.ConflictFiles[1] != "dir/b.go" {
		t.Errorf("ConflictFiles = %v, want both conflicted paths", result.ConflictFiles)
	}
}

func TestGitOperation_ReturnsResultAndErrorOnOtherHTTPErrors(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusInternalServerError,
		`{"success":false,"operation":"pull","error":"remote unreachable"}`))

	result, err := newHTTPOnlyClient(srv.URL).GitPull(context.Background(), false, "")
	if err == nil {
		t.Fatal("GitPull returned nil error for 500")
	}
	if !strings.Contains(err.Error(), "git operation failed with status 500") {
		t.Errorf("error = %v, want status 500 named", err)
	}
	if !strings.Contains(err.Error(), "remote unreachable") {
		t.Errorf("error = %v, want the server error message included", err)
	}
	if result == nil || result.Error != "remote unreachable" {
		t.Errorf("result = %+v, want the decoded body alongside the error", result)
	}
}

func TestGitOperation_RejectsMalformedBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"success":`))

	_, err := newHTTPOnlyClient(srv.URL).GitPull(context.Background(), false, "")
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("error = %v, want parse failure", err)
	}
}

func TestGitOperation_HonoursContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, gitOperationOK))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newHTTPOnlyClient(srv.URL).GitPush(ctx, false, false, "")
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestGitCreatePR_PostsFullPayloadAndDecodesResult(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,"branch_pushed":true,
		"pr_url":"https://github.com/kdlbs/kandev/pull/1",
		"provider":"github","output":"created",
		"error_code":"empty_remote_branch_publish_failed"
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GitCreatePR(
		context.Background(), "feat: thing", "body text", "main", true, "svc")
	if err != nil {
		t.Fatalf("GitCreatePR: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/git/create-pr" {
		t.Errorf("request = %s %s, want POST /api/v1/git/create-pr", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent struct {
		Title      string `json:"title"`
		Body       string `json:"body"`
		BaseBranch string `json:"base_branch"`
		Draft      bool   `json:"draft"`
		Repo       string `json:"repo"`
	}
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Title != "feat: thing" || sent.Body != "body text" {
		t.Errorf("sent title/body = %q / %q", sent.Title, sent.Body)
	}
	if sent.BaseBranch != "main" || !sent.Draft || sent.Repo != "svc" {
		t.Errorf("sent = %+v, want main/draft/svc", sent)
	}

	if !result.Success || !result.BranchPushed {
		t.Errorf("result = %+v, want success with branch pushed", result)
	}
	if result.PRURL != "https://github.com/kdlbs/kandev/pull/1" {
		t.Errorf("PRURL = %q", result.PRURL)
	}
	if result.Provider != "github" || result.Output != "created" {
		t.Errorf("provider/output = %q / %q, want github / created", result.Provider, result.Output)
	}
	if result.ErrorCode != "empty_remote_branch_publish_failed" {
		t.Errorf("error code = %q, want empty_remote_branch_publish_failed", result.ErrorCode)
	}
}

func TestGitCreatePR_TreatsConflictAsNonError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusConflict,
		`{"success":false,"error":"a PR operation is already running"}`))

	result, err := newHTTPOnlyClient(srv.URL).GitCreatePR(
		context.Background(), "t", "b", "main", false, "")
	if err != nil {
		t.Fatalf("GitCreatePR on 409 returned error %v, want the result with no error", err)
	}
	if result.Success || result.Error != "a PR operation is already running" {
		t.Errorf("result = %+v, want the decoded in-progress body", result)
	}
}

func TestGitCreatePR_ReturnsErrorOnOtherHTTPErrors(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusForbidden,
		`{"success":false,"error":"gh not authenticated"}`))

	result, err := newHTTPOnlyClient(srv.URL).GitCreatePR(
		context.Background(), "t", "b", "main", false, "")
	if err == nil || !strings.Contains(err.Error(), "create PR failed with status 403") {
		t.Fatalf("error = %v, want status 403 named", err)
	}
	if result == nil || result.Error != "gh not authenticated" {
		t.Errorf("result = %+v, want the decoded body alongside the error", result)
	}
}

func TestGitShowCommit_PutsSHAInPathAndDecodesStats(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,"commit_sha":"abc123","message":"fix: thing",
		"author":"Dev <dev@example.com>","date":"2026-08-10T10:00:00Z",
		"files":{"a.go":{"additions":3}},
		"files_changed":1,"insertions":3,"deletions":1
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GitShowCommit(context.Background(), "abc123", "my svc")
	if err != nil {
		t.Fatalf("GitShowCommit: %v", err)
	}

	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if got.Path != "/api/v1/git/commit/abc123" {
		t.Errorf("path = %q, want the SHA as the last path segment", got.Path)
	}
	if repo := got.Query.Get("repo"); repo != "my svc" {
		t.Errorf("repo = %q, want %q", repo, "my svc")
	}

	if !result.Success || result.CommitSHA != "abc123" {
		t.Errorf("result = %+v, want abc123", result)
	}
	if result.Message != "fix: thing" || result.Author != "Dev <dev@example.com>" {
		t.Errorf("message/author = %q / %q", result.Message, result.Author)
	}
	if result.Date != "2026-08-10T10:00:00Z" {
		t.Errorf("Date = %q", result.Date)
	}
	if result.FilesChanged != 1 || result.Insertions != 3 || result.Deletions != 1 {
		t.Errorf("stats = %d/%d/%d, want 1/3/1",
			result.FilesChanged, result.Insertions, result.Deletions)
	}
	if _, ok := result.Files["a.go"]; !ok {
		t.Errorf("Files = %v, want an a.go entry", result.Files)
	}
}

func TestGitShowCommit_OmitsRepoQueryWhenEmpty(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true,"commit_sha":"a"}`))

	if _, err := newHTTPOnlyClient(srv.URL).GitShowCommit(context.Background(), "a", ""); err != nil {
		t.Fatalf("GitShowCommit: %v", err)
	}
	if got.RawQuery != "" {
		t.Errorf("raw query = %q, want empty for single-repo call", got.RawQuery)
	}
}

func TestGitShowCommit_ReturnsErrorOnHTTPError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusNotFound,
		`{"success":false,"error":"unknown revision"}`))

	result, err := newHTTPOnlyClient(srv.URL).GitShowCommit(context.Background(), "nope", "")
	if err == nil || !strings.Contains(err.Error(), "git show commit failed with status 404") {
		t.Fatalf("error = %v, want status 404 named", err)
	}
	if result == nil || result.Error != "unknown revision" {
		t.Errorf("result = %+v, want the decoded body alongside the error", result)
	}
}

func TestGitLog_BuildsFullQueryAndDecodesCommits(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,
		"commits":[{
			"commit_sha":"aaa","parent_sha":"bbb","commit_message":"feat: x",
			"author_name":"Dev","author_email":"dev@example.com",
			"committed_at":"2026-08-10T09:00:00Z",
			"files_changed":2,"insertions":10,"deletions":4,
			"pushed":true,"repository_name":"backend"
		}]
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GitLog(
		context.Background(), "base sha", 50, "feature/x", "my svc")
	if err != nil {
		t.Fatalf("GitLog: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/git/log" {
		t.Errorf("request = %s %s, want GET /api/v1/git/log", got.Method, got.Path)
	}
	if since := got.Query.Get("since"); since != "base sha" {
		t.Errorf("since = %q, want %q", since, "base sha")
	}
	if limit := got.Query.Get("limit"); limit != "50" {
		t.Errorf("limit = %q, want 50", limit)
	}
	if tb := got.Query.Get("target_branch"); tb != "feature/x" {
		t.Errorf("target_branch = %q, want feature/x", tb)
	}
	if repo := got.Query.Get("repo"); repo != "my svc" {
		t.Errorf("repo = %q, want %q", repo, "my svc")
	}

	if !result.Success {
		t.Error("Success = false, want true")
	}
	if len(result.Commits) != 1 {
		t.Fatalf("Commits = %d, want 1", len(result.Commits))
	}
	commit := result.Commits[0]
	if commit.CommitSHA != "aaa" || commit.ParentSHA != "bbb" {
		t.Errorf("sha/parent = %q / %q, want aaa / bbb", commit.CommitSHA, commit.ParentSHA)
	}
	if commit.CommitMessage != "feat: x" {
		t.Errorf("CommitMessage = %q", commit.CommitMessage)
	}
	if commit.AuthorName != "Dev" || commit.AuthorEmail != "dev@example.com" {
		t.Errorf("author = %q <%q>", commit.AuthorName, commit.AuthorEmail)
	}
	if commit.CommittedAt != "2026-08-10T09:00:00Z" {
		t.Errorf("CommittedAt = %q", commit.CommittedAt)
	}
	if commit.FilesChanged != 2 || commit.Insertions != 10 || commit.Deletions != 4 {
		t.Errorf("stats = %d/%d/%d, want 2/10/4",
			commit.FilesChanged, commit.Insertions, commit.Deletions)
	}
	if !commit.Pushed {
		t.Error("Pushed = false, want true")
	}
	if commit.RepositoryName != "backend" {
		t.Errorf("RepositoryName = %q, want backend", commit.RepositoryName)
	}
}

func TestGitLog_OmitsOptionalQueryParams(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true,"commits":[]}`))

	if _, err := newHTTPOnlyClient(srv.URL).GitLog(context.Background(), "", 10, "", ""); err != nil {
		t.Fatalf("GitLog: %v", err)
	}
	for _, key := range []string{"target_branch", "repo"} {
		if _, present := got.Query[key]; present {
			t.Errorf("%s query present when empty: %q", key, got.RawQuery)
		}
	}
	// since is always sent, even empty — the server distinguishes "" from absent.
	if _, present := got.Query["since"]; !present {
		t.Errorf("since query absent: %q", got.RawQuery)
	}
}

func TestGitLog_DecodesPerRepoErrorsFromPartialFanOut(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,"commits":[],
		"per_repo_errors":[
			{"repository_name":"docs","error":"not a git repository"},
			{"repository_name":"web","error":"detached HEAD"}
		]
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GitLog(context.Background(), "", 10, "", "")
	if err != nil {
		t.Fatalf("GitLog: %v", err)
	}
	if len(result.PerRepoErrors) != 2 {
		t.Fatalf("PerRepoErrors = %d, want 2", len(result.PerRepoErrors))
	}
	if result.PerRepoErrors[0].RepositoryName != "docs" ||
		result.PerRepoErrors[0].Error != "not a git repository" {
		t.Errorf("PerRepoErrors[0] = %+v", result.PerRepoErrors[0])
	}
	if result.PerRepoErrors[1].RepositoryName != "web" ||
		result.PerRepoErrors[1].Error != "detached HEAD" {
		t.Errorf("PerRepoErrors[1] = %+v", result.PerRepoErrors[1])
	}
}

func TestGitLog_ReturnsErrorOnHTTPError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusBadRequest,
		`{"success":false,"error":"invalid limit"}`))

	result, err := newHTTPOnlyClient(srv.URL).GitLog(context.Background(), "", -1, "", "")
	if err == nil || !strings.Contains(err.Error(), "git log failed with status 400") {
		t.Fatalf("error = %v, want status 400 named", err)
	}
	if result == nil || result.Error != "invalid limit" {
		t.Errorf("result = %+v, want the decoded body alongside the error", result)
	}
}

func TestGetCumulativeDiff_BuildsQueryAndDecodesTruncation(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,"base_commit":"base1","head_commit":"head1",
		"total_commits":7,"files":{"a.go":{"additions":1}},
		"truncated_files_count":12
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GetCumulativeDiff(
		context.Background(), "base sha", "feature/x")
	if err != nil {
		t.Fatalf("GetCumulativeDiff: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/git/cumulative-diff" {
		t.Errorf("request = %s %s, want GET /api/v1/git/cumulative-diff", got.Method, got.Path)
	}
	if base := got.Query.Get("base"); base != "base sha" {
		t.Errorf("base = %q, want %q", base, "base sha")
	}
	if tb := got.Query.Get("target_branch"); tb != "feature/x" {
		t.Errorf("target_branch = %q, want feature/x", tb)
	}

	if !result.Success {
		t.Error("Success = false, want true")
	}
	if result.BaseCommit != "base1" || result.HeadCommit != "head1" {
		t.Errorf("base/head = %q / %q, want base1 / head1", result.BaseCommit, result.HeadCommit)
	}
	if result.TotalCommits != 7 {
		t.Errorf("TotalCommits = %d, want 7", result.TotalCommits)
	}
	if result.TruncatedFilesCount != 12 {
		t.Errorf("TruncatedFilesCount = %d, want 12 — the UI's hidden-files banner reads this",
			result.TruncatedFilesCount)
	}
	if _, ok := result.Files["a.go"]; !ok {
		t.Errorf("Files = %v, want an a.go entry", result.Files)
	}
}

func TestGetCumulativeDiff_OmitsTargetBranchWhenEmpty(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))

	if _, err := newHTTPOnlyClient(srv.URL).GetCumulativeDiff(context.Background(), "b", ""); err != nil {
		t.Fatalf("GetCumulativeDiff: %v", err)
	}
	if _, present := got.Query["target_branch"]; present {
		t.Errorf("target_branch present when empty: %q", got.RawQuery)
	}
}

func TestGetCumulativeDiff_ReturnsErrorOnHTTPError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusInternalServerError,
		`{"success":false,"error":"diff too large"}`))

	result, err := newHTTPOnlyClient(srv.URL).GetCumulativeDiff(context.Background(), "b", "")
	if err == nil || !strings.Contains(err.Error(), "cumulative diff failed with status 500") {
		t.Fatalf("error = %v, want status 500 named", err)
	}
	if result == nil || result.Error != "diff too large" {
		t.Errorf("result = %+v, want the decoded body alongside the error", result)
	}
}

// gitStatusBody exercises every field of GitStatusResult.
const gitStatusBody = `{
	"success":true,"is_submodule":true,
	"branch":"feature/x","remote_branch":"origin/feature/x",
	"head_commit":"head1","base_commit":"base1",
	"ahead":3,"behind":1,"remote_ahead":4,"remote_behind":2,
	"remote_head_commit":"remote1",
	"modified":["m.go"],"added":["a.go"],"deleted":["d.go"],
	"untracked":["u.go"],"renamed":["r.go"],
	"files":{"m.go":{"status":"M"}},
	"timestamp":"2026-08-10T11:00:00Z",
	"branch_additions":25,"branch_deletions":7
}`

func assertFullGitStatus(t *testing.T, result *GitStatusResult) {
	t.Helper()
	if !result.Success || !result.IsSubmodule {
		t.Errorf("success/is_submodule = %v / %v, want true / true", result.Success, result.IsSubmodule)
	}
	if result.Branch != "feature/x" || result.RemoteBranch != "origin/feature/x" {
		t.Errorf("branch/remote = %q / %q", result.Branch, result.RemoteBranch)
	}
	if result.HeadCommit != "head1" || result.BaseCommit != "base1" {
		t.Errorf("head/base = %q / %q", result.HeadCommit, result.BaseCommit)
	}
	if result.Ahead != 3 || result.Behind != 1 {
		t.Errorf("ahead/behind = %d / %d, want 3 / 1", result.Ahead, result.Behind)
	}
	if result.RemoteAhead != 4 || result.RemoteBehind != 2 || result.RemoteHeadCommit != "remote1" {
		t.Errorf("remote ancestry = %d / %d / %q, want 4 / 2 / remote1",
			result.RemoteAhead, result.RemoteBehind, result.RemoteHeadCommit)
	}
	for name, got := range map[string][]string{
		"modified":  result.Modified,
		"added":     result.Added,
		"deleted":   result.Deleted,
		"untracked": result.Untracked,
		"renamed":   result.Renamed,
	} {
		if len(got) != 1 {
			t.Errorf("%s = %v, want exactly one path", name, got)
		}
	}
	if result.Modified[0] != "m.go" || result.Untracked[0] != "u.go" {
		t.Errorf("modified/untracked = %v / %v", result.Modified, result.Untracked)
	}
	if _, ok := result.Files["m.go"]; !ok {
		t.Errorf("Files = %v, want an m.go entry", result.Files)
	}
	if result.Timestamp != "2026-08-10T11:00:00Z" {
		t.Errorf("Timestamp = %q", result.Timestamp)
	}
	if result.BranchAdditions != 25 || result.BranchDeletions != 7 {
		t.Errorf("branch additions/deletions = %d / %d, want 25 / 7",
			result.BranchAdditions, result.BranchDeletions)
	}
}

func TestGetGitStatus_UsesCachedEndpointAndDecodesEveryField(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, gitStatusBody))

	result, err := newHTTPOnlyClient(srv.URL).GetGitStatus(context.Background())
	if err != nil {
		t.Fatalf("GetGitStatus: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/git/status" {
		t.Errorf("request = %s %s, want GET /api/v1/git/status", got.Method, got.Path)
	}
	if got.RawQuery != "" {
		t.Errorf("raw query = %q, want empty — the cached path must not send fresh=true", got.RawQuery)
	}
	assertFullGitStatus(t, result)
}

func TestGetGitStatusFresh_SendsFreshQueryParam(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, gitStatusBody))

	result, err := newHTTPOnlyClient(srv.URL).GetGitStatusFresh(context.Background())
	if err != nil {
		t.Fatalf("GetGitStatusFresh: %v", err)
	}

	if got.Path != "/api/v1/git/status" {
		t.Errorf("path = %q, want /api/v1/git/status", got.Path)
	}
	if fresh := got.Query.Get("fresh"); fresh != "true" {
		t.Errorf("fresh = %q, want true — without it the server serves the poll-loop cache", fresh)
	}
	assertFullGitStatus(t, result)
}

func TestGetGitStatusMultiFresh_UsesMultiEndpointAndDecodesPerRepoEntries(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,
		"repos":[
			{"repository_name":"backend","status":{"success":true,"branch":"main","ahead":2}},
			{"repository_name":"web","status":{"success":true,"branch":"feature/y","behind":5}}
		]
	}`))

	result, err := newHTTPOnlyClient(srv.URL).GetGitStatusMultiFresh(context.Background())
	if err != nil {
		t.Fatalf("GetGitStatusMultiFresh: %v", err)
	}

	if got.Path != "/api/v1/git/status/multi" {
		t.Errorf("path = %q, want /api/v1/git/status/multi", got.Path)
	}
	if fresh := got.Query.Get("fresh"); fresh != "true" {
		t.Errorf("fresh = %q, want true", fresh)
	}

	if !result.Success {
		t.Error("Success = false, want true")
	}
	if len(result.Repos) != 2 {
		t.Fatalf("Repos = %d, want 2", len(result.Repos))
	}
	if result.Repos[0].RepositoryName != "backend" ||
		result.Repos[0].Status.Branch != "main" || result.Repos[0].Status.Ahead != 2 {
		t.Errorf("Repos[0] = %+v, want backend on main ahead 2", result.Repos[0])
	}
	if result.Repos[1].RepositoryName != "web" ||
		result.Repos[1].Status.Branch != "feature/y" || result.Repos[1].Status.Behind != 5 {
		t.Errorf("Repos[1] = %+v, want web on feature/y behind 5", result.Repos[1])
	}
}

func TestGitStatusEndpoints_ReturnResultAndErrorOnHTTPError(t *testing.T) {
	tests := []struct {
		name    string
		call    func(c *Client) (string, error)
		wantMsg string
	}{
		{
			name: "cached status",
			call: func(c *Client) (string, error) {
				r, err := c.GetGitStatus(context.Background())
				if r == nil {
					return "", err
				}
				return r.Error, err
			},
			wantMsg: "git status failed with status 500",
		},
		{
			name: "fresh status",
			call: func(c *Client) (string, error) {
				r, err := c.GetGitStatusFresh(context.Background())
				if r == nil {
					return "", err
				}
				return r.Error, err
			},
			wantMsg: "git status fresh failed with status 500",
		},
		{
			name: "multi status",
			call: func(c *Client) (string, error) {
				r, err := c.GetGitStatusMultiFresh(context.Background())
				if r == nil {
					return "", err
				}
				return r.Error, err
			},
			wantMsg: "git status multi failed with status 500",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(http.StatusInternalServerError,
				`{"success":false,"error":"worktree gone"}`))

			bodyErr, err := tc.call(newHTTPOnlyClient(srv.URL))
			if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error = %v, want %q", err, tc.wantMsg)
			}
			if bodyErr != "worktree gone" {
				t.Errorf("result.Error = %q, want the decoded body alongside the error", bodyErr)
			}
		})
	}
}

// fetchJSONResult is shared by all three status endpoints; a decode failure
// must return nil (not a partially-filled result) so callers can't mistake
// garbage for a real status.
func TestGitStatusEndpoints_ReturnNilResultOnMalformedBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"branch":`))

	result, err := newHTTPOnlyClient(srv.URL).GetGitStatus(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("error = %v, want parse failure", err)
	}
	if result != nil {
		t.Errorf("result = %+v, want nil on decode failure", result)
	}
}

func TestGitStatus_HonoursContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, gitStatusBody))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := newHTTPOnlyClient(srv.URL).GetGitStatus(ctx); err == nil ||
		!strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v, want context canceled", err)
	}
}
