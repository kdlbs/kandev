package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPATClient_SubmitReview_SendsEventAndOptionalBody(t *testing.T) {
	cases := []struct {
		name     string
		event    string
		body     string
		wantJSON map[string]string
	}{
		{"approve without a body", "APPROVE", "", map[string]string{"event": "APPROVE"}},
		{
			"request changes with a body", "REQUEST_CHANGES", "please fix",
			map[string]string{"event": "REQUEST_CHANGES", "body": "please fix"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
				return http.StatusOK, `{}`
			})
			if err := c.SubmitReview(context.Background(), "acme", "widget", 42, tc.event, tc.body); err != nil {
				t.Fatalf("SubmitReview: %v", err)
			}
			got := (*requests)[0]
			if got.Method != http.MethodPost {
				t.Errorf("method = %q, want POST", got.Method)
			}
			if got.Path != "/repos/acme/widget/pulls/42/reviews" {
				t.Errorf("path = %q", got.Path)
			}
			if got.Header.Get("Content-Type") != "application/json" {
				t.Errorf("content-type = %q, want application/json", got.Header.Get("Content-Type"))
			}
			var payload map[string]string
			if err := json.Unmarshal([]byte(got.Body), &payload); err != nil {
				t.Fatalf("decode request body %q: %v", got.Body, err)
			}
			if len(payload) != len(tc.wantJSON) {
				t.Fatalf("payload = %#v, want %#v", payload, tc.wantJSON)
			}
			for k, v := range tc.wantJSON {
				if payload[k] != v {
					t.Errorf("payload[%q] = %q, want %q", k, payload[k], v)
				}
			}
		})
	}
}

func TestPATClient_SubmitReview_ErrorIncludesStatusAndBody(t *testing.T) {
	c, _ := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
		return http.StatusUnprocessableEntity, `{"message":"Can not approve your own pull request"}`
	})
	err := c.SubmitReview(context.Background(), "acme", "widget", 42, "APPROVE", "")
	if err == nil {
		t.Fatal("expected an error from 422")
	}
	if !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "own pull request") {
		t.Errorf("err = %v, want the status and upstream body surfaced", err)
	}
}

func TestPATClient_MergePR(t *testing.T) {
	t.Run("sends the merge method", func(t *testing.T) {
		c, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
			return http.StatusOK, `{"status":"merged"}`
		})
		if _, err := c.MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"}); err != nil {
			t.Fatalf("MergePR: %v", err)
		}
		got := (*requests)[0]
		if got.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", got.Method)
		}
		if got.Path != "/repos/acme/widget/pulls/42/merge-async" {
			t.Errorf("path = %q", got.Path)
		}
		if got.Body != `{"merge_action":"default","merge_method":"squash"}` {
			t.Errorf("body = %q, want queue-aware merge payload", got.Body)
		}
	})
	t.Run("omits the merge method when unset", func(t *testing.T) {
		c, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
			return http.StatusOK, `{"status":"merged"}`
		})
		if _, err := c.MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{}); err != nil {
			t.Fatalf("MergePR: %v", err)
		}
		if (*requests)[0].Body != `{"merge_action":"default"}` {
			t.Errorf("body = %q, want GitHub to choose direct or queued merge",
				(*requests)[0].Body)
		}
	})
	t.Run("sends the expected head SHA", func(t *testing.T) {
		c, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
			return http.StatusOK, `{"status":"merged"}`
		})
		request := MergePRRequest{MergeMethod: "squash", ExpectedHeadSHA: "abc123"}
		if _, err := c.MergePR(context.Background(), "acme", "widget", 42, request); err != nil {
			t.Fatalf("MergePR: %v", err)
		}
		if got := (*requests)[0].Body; got != `{"merge_action":"default","merge_method":"squash","sha":"abc123"}` {
			t.Errorf("body = %q, want expected head SHA", got)
		}
	})
}

// A merge rejection must stay recoverable as *GitHubAPIError so the HTTP
// layer can map 405 (not mergeable) and 409 (conflict) onto a 409 response.
func TestPATClient_MergePR_StatusIsRecoverable(t *testing.T) {
	for _, status := range []int{http.StatusMethodNotAllowed, http.StatusConflict} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, _ := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
				return status, `{"message":"Pull Request is not mergeable"}`
			})
			_, err := c.MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "merge"})
			var apiErr *GitHubAPIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want a *GitHubAPIError", err)
			}
			if apiErr.StatusCode != status {
				t.Errorf("status = %d, want %d", apiErr.StatusCode, status)
			}
			if apiErr.Endpoint != "/repos/acme/widget/pulls/42/merge-async" {
				t.Errorf("endpoint = %q", apiErr.Endpoint)
			}
			if !strings.Contains(apiErr.Body, "not mergeable") {
				t.Errorf("body = %q, want the upstream message", apiErr.Body)
			}
		})
	}
}

func TestPATClient_MergePR_AlreadyQueuedIsIdempotent(t *testing.T) {
	c, _ := newRecordingPATServerFunc(t, func(r *http.Request) (int, string) {
		if r.Method == http.MethodGet {
			return http.StatusOK, `{"status":"enqueued","details":{"message":"Pull request was added to the merge queue."}}`
		}
		return http.StatusConflict, `{"status":"pending","details":{"message":"Merge request enqueued.","uuid":"request-1","merge_method":"squash","merge_action":"default","expected_head_sha":"abc123"}}`
	})
	outcome, err := c.MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"})
	if err != nil || outcome != MergeOutcomeQueued {
		t.Fatalf("outcome = %q, err = %v, want queued", outcome, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.3
func TestPATClient_MergePR_PollsNestedPendingResponse(t *testing.T) {
	c, requests := newRecordingPATServerFunc(t, func(r *http.Request) (int, string) {
		if r.Method == http.MethodGet {
			return http.StatusOK, `{"status":"merged","details":{"sha":"abc123"}}`
		}
		return http.StatusAccepted, `{"status":"pending","details":{"message":"Merge request enqueued.","uuid":"request-1","merge_method":"squash","merge_action":"default","expected_head_sha":"abc123"}}`
	})

	outcome, err := c.MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"})
	if err != nil || outcome != MergeOutcomeMerged {
		t.Fatalf("outcome = %q, err = %v, want merged", outcome, err)
	}
	if len(*requests) != 2 || (*requests)[1].Path != "/repos/acme/widget/pulls/42/merge-async/request-1" {
		t.Fatalf("requests = %#v, want a poll for request-1", *requests)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.7
func TestPATClient_MergePR_PendingPollHonorsContextBeforeNextRequest(t *testing.T) {
	c, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
		return http.StatusAccepted, `{"status":"pending","details":{"uuid":"request-1"}}`
	})
	var gotDelay time.Duration
	c.mergePollWait = func(_ context.Context, delay time.Duration) error {
		gotDelay = delay
		return context.DeadlineExceeded
	}

	_, err := c.MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("MergePR error = %v, want context deadline", err)
	}
	if got := len(*requests); got != 1 {
		t.Fatalf("request count = %d, want only the initial request before cancellation", got)
	}
	if gotDelay != time.Second {
		t.Fatalf("poll delay = %s, want one second", gotDelay)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.13
func TestPATClient_MergePR_ReportsNestedFailureMessage(t *testing.T) {
	c, _ := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
		return http.StatusOK, `{"status":"failed","details":{"message":"Branch protection blocked the merge.","expected_head_sha":"abc123"}}`
	})

	_, err := c.MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"})
	if err == nil || !strings.Contains(err.Error(), "Branch protection blocked the merge.") ||
		!strings.Contains(err.Error(), "abc123") {
		t.Fatalf("err = %v, want the nested provider message and expected head", err)
	}
}

func TestPATClient_CreateGist(t *testing.T) {
	c, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
		return http.StatusCreated, `{"id":"abc123","html_url":"https://gist.github.com/abc123",
			"created_at":"2026-07-01T12:00:00Z"}`
	})

	resp, err := c.CreateGist(context.Background(), CreateGistInput{
		Description: "kandev share",
		Public:      false,
		Files:       map[string]GistFile{"README.md": {Content: "# hello"}},
	})
	if err != nil {
		t.Fatalf("CreateGist: %v", err)
	}
	if resp.ID != "abc123" || resp.HTMLURL != "https://gist.github.com/abc123" {
		t.Errorf("response = %#v", resp)
	}
	if !resp.CreatedAt.Equal(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("created_at = %v", resp.CreatedAt)
	}
	got := (*requests)[0]
	if got.Method != http.MethodPost || got.Path != "/gists" {
		t.Errorf("request = %s %s, want POST /gists", got.Method, got.Path)
	}
	var payload struct {
		Description string                       `json:"description"`
		Public      bool                         `json:"public"`
		Files       map[string]map[string]string `json:"files"`
	}
	if err := json.Unmarshal([]byte(got.Body), &payload); err != nil {
		t.Fatalf("decode payload %q: %v", got.Body, err)
	}
	if payload.Description != "kandev share" || payload.Public {
		t.Errorf("payload meta = %#v", payload)
	}
	if payload.Files["README.md"]["content"] != "# hello" {
		t.Errorf("payload files = %#v", payload.Files)
	}
}

func TestPATClient_CreateGist_ErrorIsTyped(t *testing.T) {
	c, _ := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
		return http.StatusUnauthorized, `{"message":"Bad credentials"}`
	})
	resp, err := c.CreateGist(context.Background(), CreateGistInput{Files: map[string]GistFile{}})
	if resp != nil {
		t.Errorf("resp = %#v, want nil", resp)
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want a *GitHubAPIError with 401", err)
	}
}

func TestPATClient_DeleteGist(t *testing.T) {
	t.Run("deletes by id", func(t *testing.T) {
		c, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
			return http.StatusNoContent, ""
		})
		if err := c.DeleteGist(context.Background(), "abc123"); err != nil {
			t.Fatalf("DeleteGist: %v", err)
		}
		got := (*requests)[0]
		if got.Method != http.MethodDelete || got.Path != "/gists/abc123" {
			t.Errorf("request = %s %s, want DELETE /gists/abc123", got.Method, got.Path)
		}
		if got.Header.Get("Authorization") != "token test-token" {
			t.Errorf("DELETE must still carry auth, got %q", got.Header.Get("Authorization"))
		}
	})
	t.Run("empty id never reaches the network", func(t *testing.T) {
		c, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
			return http.StatusNoContent, ""
		})
		if err := c.DeleteGist(context.Background(), ""); err == nil {
			t.Fatal("expected an error for an empty gist id")
		}
		if len(*requests) != 0 {
			t.Errorf("requests = %d, want 0", len(*requests))
		}
	})
	// share.IsAlreadyGone matches on the typed 404 to treat an already-revoked
	// gist as a soft success rather than a 502.
	t.Run("404 stays recoverable as a typed error", func(t *testing.T) {
		c, _ := newRecordingPATServerFunc(t, func(*http.Request) (int, string) {
			return http.StatusNotFound, `{"message":"Not Found"}`
		})
		err := c.DeleteGist(context.Background(), "gone")
		var apiErr *GitHubAPIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
			t.Fatalf("err = %v, want a *GitHubAPIError with 404", err)
		}
		if apiErr.Endpoint != "/gists/gone" {
			t.Errorf("endpoint = %q, want /gists/gone", apiErr.Endpoint)
		}
	})
}

func TestPATClient_ListPRReviews(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/pulls/42/reviews": `[
			{"id":1,"state":"APPROVED","body":"lgtm","submitted_at":"2026-01-05T10:00:00Z",
			 "user":{"login":"alice","avatar_url":"https://avatars/alice"}},
			{"id":2,"state":"CHANGES_REQUESTED","body":"nope","submitted_at":"2026-01-06T10:00:00Z",
			 "user":{"login":"bob","avatar_url":""}}
		]`,
	})

	reviews, err := c.ListPRReviews(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("ListPRReviews: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("reviews = %d, want 2", len(reviews))
	}
	want := PRReview{
		ID: 1, Author: "alice", AuthorAvatar: "https://avatars/alice",
		State: "APPROVED", Body: "lgtm",
		CreatedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC),
	}
	if reviews[0] != want {
		t.Errorf("reviews[0] = %#v, want %#v", reviews[0], want)
	}
	if reviews[1].ID != 2 || reviews[1].Author != "bob" || reviews[1].State != "CHANGES_REQUESTED" {
		t.Errorf("reviews[1] = %#v", reviews[1])
	}
	if (*requests)[0].Path != "/repos/acme/widget/pulls/42/reviews" {
		t.Errorf("path = %q", (*requests)[0].Path)
	}
}

func TestPATClient_ListPRComments_MergesReviewAndIssueCommentsInTimeOrder(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/pulls/42/comments": `[
			{"id":20,"path":"main.go","line":7,"side":"RIGHT","body":"inline note",
			 "created_at":"2026-01-05T12:00:00Z","updated_at":"2026-01-05T12:30:00Z",
			 "in_reply_to_id":19,"user":{"login":"alice","avatar_url":"https://a","type":"User"}}
		]`,
		"/repos/acme/widget/issues/42/comments": `[
			{"id":10,"body":"conversation note","created_at":"2026-01-05T09:00:00Z",
			 "updated_at":"2026-01-05T09:00:00Z","user":{"login":"dependabot","avatar_url":"https://d","type":"Bot"}}
		]`,
	})

	comments, err := c.ListPRComments(context.Background(), "acme", "widget", 42, nil)
	if err != nil {
		t.Fatalf("ListPRComments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("comments = %d, want 2", len(comments))
	}
	// The issue comment is older, so it must sort first.
	first, second := comments[0], comments[1]
	if first.ID != 10 || first.CommentType != commentTypeIssue {
		t.Errorf("comments[0] = %#v, want the older issue comment", first)
	}
	if !first.AuthorIsBot {
		t.Error("a Bot-typed user must set AuthorIsBot")
	}
	if second.ID != 20 || second.CommentType != commentTypeReview {
		t.Errorf("comments[1] = %#v, want the review comment", second)
	}
	if second.Path != "main.go" || second.Line != 7 || second.Side != "RIGHT" {
		t.Errorf("review comment location = %#v", second)
	}
	if second.InReplyTo == nil || *second.InReplyTo != 19 {
		t.Errorf("in_reply_to = %v, want 19", second.InReplyTo)
	}
	if second.AuthorIsBot {
		t.Error("a User-typed author must not set AuthorIsBot")
	}
	for _, req := range *requests {
		if !strings.Contains(req.Query, "per_page=100") {
			t.Errorf("%s query = %q, want per_page=100", req.Path, req.Query)
		}
		if strings.Contains(req.Query, "since=") {
			t.Errorf("%s query = %q, want no since when the caller passed nil", req.Path, req.Query)
		}
	}
}

func TestPATClient_ListPRComments_AppendsSinceToBothEndpoints(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/pulls/42/comments":  `[]`,
		"/repos/acme/widget/issues/42/comments": `[]`,
	})
	since := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := c.ListPRComments(context.Background(), "acme", "widget", 42, &since); err != nil {
		t.Fatalf("ListPRComments: %v", err)
	}
	if len(*requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(*requests))
	}
	for _, req := range *requests {
		if got := parseQueryValues(t, req.Query).Get("since"); got != "2026-01-02T03:04:05Z" {
			t.Errorf("%s since = %q, want the RFC3339 timestamp", req.Path, got)
		}
	}
}

// The issue-comment leg runs after the review leg; its failure must abort the
// whole call rather than silently returning a half-populated list.
func TestPATClient_ListPRComments_IssueLegFailureAborts(t *testing.T) {
	c, _ := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/pulls/42/comments": `[]`,
	})
	comments, err := c.ListPRComments(context.Background(), "acme", "widget", 42, nil)
	if err == nil {
		t.Fatal("expected an error when the issue-comments call fails")
	}
	if comments != nil {
		t.Errorf("comments = %#v, want nil", comments)
	}
}

func TestPATClient_ListPRFiles(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/pulls/42/files": `[
			{"filename":"main.go","status":"modified","additions":4,"deletions":1,"patch":"@@ -1 +1 @@\n-old\n+new"},
			{"filename":"new.go","previous_filename":"old.go","status":"renamed","additions":0,"deletions":0}
		]`,
	})

	files, err := c.ListPRFiles(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("ListPRFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[0].Filename != "main.go" || files[0].Status != "modified" {
		t.Errorf("files[0] identity = %#v", files[0])
	}
	if files[0].Additions != 4 || files[0].Deletions != 1 {
		t.Errorf("files[0] stats = (%d, %d), want (4, 1)", files[0].Additions, files[0].Deletions)
	}
	if !strings.Contains(files[0].Patch, "+new") {
		t.Errorf("files[0] patch = %q, want the hunk body preserved", files[0].Patch)
	}
	if files[1].OldPath != "old.go" || files[1].Status != "renamed" {
		t.Errorf("files[1] = %#v, want the rename source preserved", files[1])
	}
	if !strings.Contains((*requests)[0].Query, "per_page=100") {
		t.Errorf("query = %q, want per_page=100", (*requests)[0].Query)
	}
}

func TestPATClient_ListPRFiles_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	files, err := c.ListPRFiles(context.Background(), "acme", "widget", 42)
	if files != nil {
		t.Errorf("files = %#v, want nil", files)
	}
	if err == nil || !strings.Contains(err.Error(), "list PR files") {
		t.Fatalf("err = %v, want a wrapped list-PR-files error", err)
	}
}

func TestPATClient_ListPRCommits(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/pulls/42/commits": `[
			{"sha":"aaa","commit":{"message":"feat: add thing\n\nlong body","author":{"date":"2026-01-07T10:00:00Z"}},
			 "author":{"login":"alice"}},
			{"sha":"bbb","commit":{"message":"fix: typo","author":{"date":"2026-01-08T10:00:00Z"}}}
		]`,
	})

	commits, err := c.ListPRCommits(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("ListPRCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(commits))
	}
	if commits[0].SHA != "aaa" || commits[0].AuthorLogin != "alice" {
		t.Errorf("commits[0] identity = %#v", commits[0])
	}
	if commits[0].Message != "feat: add thing" {
		t.Errorf("commits[0] message = %q, want only the subject line", commits[0].Message)
	}
	if commits[0].AuthorDate != "2026-01-07T10:00:00Z" {
		t.Errorf("commits[0] author date = %q", commits[0].AuthorDate)
	}
	if commits[0].StatsAvailable {
		t.Error("the PR-commits list carries no per-commit stats; StatsAvailable must stay false")
	}
	// A commit with no matched GitHub account decodes with an empty login.
	if commits[1].AuthorLogin != "" || commits[1].Message != "fix: typo" {
		t.Errorf("commits[1] = %#v", commits[1])
	}
	if !strings.Contains((*requests)[0].Query, "per_page=100") {
		t.Errorf("query = %q, want per_page=100", (*requests)[0].Query)
	}
}

func TestPATClient_ListPRCommits_FollowsLinkHeaderPages(t *testing.T) {
	c, requests := newLinkPaginatedPATServer(t, "/repos/acme/widget/pulls/42/commits", []string{
		`[
			{"sha":"aaa","commit":{"message":"first","author":{"date":"2026-01-07T10:00:00Z"}}},
			{"sha":"bbb","commit":{"message":"second","author":{"date":"2026-01-08T10:00:00Z"}}}
		]`,
		`[
			{"sha":"ccc","commit":{"message":"provider head","author":{"date":"2026-01-09T10:00:00Z"}}}
		]`,
	})

	commits, err := c.ListPRCommits(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("ListPRCommits: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("commits = %d, want all pages (3)", len(commits))
	}
	if commits[2].SHA != "ccc" {
		t.Errorf("last commit = %q, want the second-page provider head", commits[2].SHA)
	}
	if len(*requests) != 2 {
		t.Fatalf("requests = %d, want 2 pages", len(*requests))
	}
}

func TestPATClient_ListPRCommits_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	if _, err := c.ListPRCommits(context.Background(), "acme", "widget", 42); err == nil ||
		!strings.Contains(err.Error(), "list PR commits") {
		t.Fatalf("err = %v, want a wrapped list-PR-commits error", err)
	}
}

func TestPATClient_ListRepoBranches_FollowsLinkHeaderPages(t *testing.T) {
	c, requests := newLinkPaginatedPATServer(t, "/repos/acme/widget/branches", []string{
		`[{"name":"main"},{"name":"develop"}]`,
		`[{"name":"release/1.0"}]`,
	})

	branches, err := c.ListRepoBranches(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("ListRepoBranches: %v", err)
	}
	want := []string{"main", "develop", "release/1.0"}
	if len(branches) != len(want) {
		t.Fatalf("branches = %#v, want %v", branches, want)
	}
	for i, name := range want {
		if branches[i].Name != name {
			t.Errorf("branches[%d] = %q, want %q", i, branches[i].Name, name)
		}
	}
	if len(*requests) != 2 {
		t.Fatalf("requests = %d, want 2 pages", len(*requests))
	}
	if !strings.Contains((*requests)[0].Query, "per_page=100") {
		t.Errorf("first query = %q, want per_page=100", (*requests)[0].Query)
	}
}

func TestPATClient_ListRepoBranches_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	branches, err := c.ListRepoBranches(context.Background(), "acme", "widget")
	if branches != nil {
		t.Errorf("branches = %#v, want nil", branches)
	}
	if err == nil || !strings.Contains(err.Error(), "list repo branches") {
		t.Fatalf("err = %v, want a wrapped list-repo-branches error", err)
	}
}

// Missing allow_* fields must read as false: a permission-gated repo response
// that omits them would otherwise let the caller pick a disallowed method and
// take a 405 from the merge endpoint.
func TestPATClient_GetRepoMergeMethods(t *testing.T) {
	cases := []struct {
		name string
		body string
		want RepoMergeMethods
	}{
		{
			"all allowed",
			`{"allow_merge_commit":true,"allow_squash_merge":true,"allow_rebase_merge":true}`,
			RepoMergeMethods{Merge: true, Squash: true, Rebase: true},
		},
		{
			"squash only",
			`{"allow_merge_commit":false,"allow_squash_merge":true,"allow_rebase_merge":false}`,
			RepoMergeMethods{Squash: true},
		},
		{
			"omitted fields read as disallowed",
			`{"full_name":"acme/widget"}`,
			RepoMergeMethods{},
		},
		{
			"explicit nulls read as disallowed",
			`{"allow_merge_commit":null,"allow_squash_merge":null,"allow_rebase_merge":null}`,
			RepoMergeMethods{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, requests := newRecordingPATServer(t, map[string]string{"/repos/acme/widget": tc.body})
			got, err := c.GetRepoMergeMethods(context.Background(), "acme", "widget")
			if err != nil {
				t.Fatalf("GetRepoMergeMethods: %v", err)
			}
			if got != tc.want {
				t.Errorf("methods = %#v, want %#v", got, tc.want)
			}
			if (*requests)[0].Path != "/repos/acme/widget" {
				t.Errorf("path = %q", (*requests)[0].Path)
			}
		})
	}
}

func TestPATClient_GetRepoMergeMethods_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	got, err := c.GetRepoMergeMethods(context.Background(), "acme", "widget")
	if got != (RepoMergeMethods{}) {
		t.Errorf("methods = %#v, want the zero value on error", got)
	}
	if err == nil || !strings.Contains(err.Error(), "get repo merge methods") {
		t.Fatalf("err = %v, want a wrapped merge-methods error", err)
	}
}
