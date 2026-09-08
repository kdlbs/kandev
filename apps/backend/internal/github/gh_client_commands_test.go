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

func TestGHClient_SearchOrgRepos(t *testing.T) {
	cases := []struct {
		name  string
		query string
		limit int
		wantQ string
		wantP string
	}{
		{"no free-text query", "", 30, "q=org:acme", "per_page=30"},
		{"with free-text query", "widget", 30, "q=org:acme widget", "per_page=30"},
		{"limit clamps to the cap", "", 900, "q=org:acme", "per_page=100"},
		{"zero limit takes the default", "", 0, "q=org:acme", "per_page=20"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := newFakeGH(t, ghResponse{
				Prefix: "api search/repositories",
				Stdout: `[{"full_name":"acme/widget","owner":{"login":"acme"},"name":"widget",
					"private":true,"default_branch":"main","description":"w",
					"pushed_at":"2026-06-01T00:00:00Z"}]`,
			})
			repos, err := NewGHClient().SearchOrgRepos(context.Background(), "acme", tc.query, tc.limit)
			if err != nil {
				t.Fatalf("SearchOrgRepos: %v", err)
			}
			argv := calls(t)[0]
			if !containsPair(argv, "-f", tc.wantQ) || !containsPair(argv, "-f", tc.wantP) {
				t.Errorf("argv = %q, want -f %q and -f %q", argv, tc.wantQ, tc.wantP)
			}
			if !containsPair(argv, "--jq", ".items") {
				t.Errorf("argv = %q, want --jq .items to unwrap the search envelope", argv)
			}
			if len(repos) != 1 {
				t.Fatalf("repos = %#v, want one", repos)
			}
			got := repos[0]
			if got.FullName != "acme/widget" || got.Owner != "acme" || got.Name != "widget" {
				t.Errorf("identity = %#v", got)
			}
			if !got.Private || got.DefaultBranch != "main" || got.Description != "w" {
				t.Errorf("attributes = %#v", got)
			}
			if got.PushedAt == nil || !got.PushedAt.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
				t.Errorf("pushed_at = %v", got.PushedAt)
			}
		})
	}
}

func TestGHClient_SearchOrgRepos_Error(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "api search/repositories", Stderr: "boom", Exit: 1})
	repos, err := NewGHClient().SearchOrgRepos(context.Background(), "acme", "", 10)
	if repos != nil {
		t.Errorf("repos = %#v, want nil", repos)
	}
	if err == nil || !strings.Contains(err.Error(), "search org repos") {
		t.Fatalf("err = %v, want a wrapped search-org-repos error", err)
	}
}

func TestGHClient_ListUserRepos_ResolvesTheLoginFirst(t *testing.T) {
	calls := newFakeGH(t,
		ghResponse{Prefix: "api user -q", Stdout: "octocat\n"},
		ghResponse{
			Prefix: "api search/repositories",
			Stdout: `[{"full_name":"octocat/demo","owner":{"login":"octocat"},"name":"demo",
				"default_branch":"main","description":"Public demo"}]`,
		},
	)

	repos, err := NewGHClient().ListUserRepos(context.Background(), "language:go", 50)
	if err != nil {
		t.Fatalf("ListUserRepos: %v", err)
	}
	recorded := calls(t)
	assertGHArgv(t, recorded, 0, []string{"api", "user", "-q", ".login"})
	if !containsPair(recorded[1], "-f", "q=user:octocat language:go") {
		t.Errorf("argv = %q, want the resolved login folded into the q qualifier", recorded[1])
	}
	if len(repos) != 1 || repos[0].FullName != "octocat/demo" {
		t.Fatalf("repos = %#v", repos)
	}
	if repos[0].DefaultBranch != "main" || repos[0].Description != "Public demo" {
		t.Errorf("repo = %#v", repos[0])
	}
}

func TestGHClient_ListUserRepos_AuthFailureSkipsTheSearch(t *testing.T) {
	calls := newFakeGH(t, ghResponse{Prefix: "api user -q", Stderr: "HTTP 401", Exit: 1})
	repos, err := NewGHClient().ListUserRepos(context.Background(), "", 20)
	if repos != nil {
		t.Errorf("repos = %#v, want nil", repos)
	}
	if err == nil || !strings.Contains(err.Error(), "list user repos") {
		t.Fatalf("err = %v, want a wrapped list-user-repos error", err)
	}
	if len(calls(t)) != 1 {
		t.Errorf("gh calls = %d, want 1 — the repo search must not run after auth fails", len(calls(t)))
	}
}

func TestGHClient_ListAccessibleRepos(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api /user/repos",
		Stdout: `[
			{"full_name":"acme/widget","owner":{"login":"acme"},"name":"widget"},
			{"full_name":"acme/gadget","owner":{"login":"acme"},"name":"gadget"}
		]`,
	})

	repos, err := NewGHClient().ListAccessibleRepos(context.Background(), "WIDGET", 5000)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "/user/repos?affiliation=owner,collaborator,organization_member&sort=pushed&per_page=100",
	})
	if len(repos) != 1 || repos[0].FullName != "acme/widget" {
		t.Fatalf("repos = %#v, want the case-insensitive full_name match only", repos)
	}
}

func TestGHClient_ListAccessibleRepos_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api /user/repos", Stderr: "boom", Exit: 1})
		repos, err := NewGHClient().ListAccessibleRepos(context.Background(), "", 20)
		if repos != nil {
			t.Errorf("repos = %#v, want nil", repos)
		}
		if err == nil || !strings.Contains(err.Error(), "list accessible repos") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api /user/repos", Stdout: "nope"})
		_, err := NewGHClient().ListAccessibleRepos(context.Background(), "", 20)
		if err == nil || !strings.Contains(err.Error(), "parse search repos") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

// The reviews endpoint carries both an `author` (GraphQL shape) and a `user`
// (REST shape); `user` wins because only it also carries the avatar.
func TestGHClient_ListPRReviews_PrefersTheRESTUserOverAuthor(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api repos/",
		Stdout: `[
			{"id":1,"state":"APPROVED","body":"lgtm","submitted_at":"2026-01-05T10:00:00Z",
			 "author":{"login":"stale-author"},
			 "user":{"login":"alice","avatar_url":"https://avatars/alice"}},
			{"id":2,"state":"COMMENTED","body":"note","submitted_at":"2026-01-06T10:00:00Z",
			 "author":{"login":"bob"}}
		]`,
	})

	reviews, err := NewGHClient().ListPRReviews(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("ListPRReviews: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "repos/acme/widget/pulls/42/reviews", "--paginate",
	})
	if len(reviews) != 2 {
		t.Fatalf("reviews = %#v, want 2", reviews)
	}
	want := PRReview{
		ID: 1, Author: "alice", AuthorAvatar: "https://avatars/alice",
		State: "APPROVED", Body: "lgtm",
		CreatedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC),
	}
	if reviews[0] != want {
		t.Errorf("reviews[0] = %#v, want %#v", reviews[0], want)
	}
	// With no `user` block the author falls back to the GraphQL login and
	// there is no avatar to carry.
	if reviews[1].Author != "bob" || reviews[1].AuthorAvatar != "" {
		t.Errorf("reviews[1] = %#v, want the author fallback with no avatar", reviews[1])
	}
}

func TestGHClient_ListPRReviews_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().ListPRReviews(context.Background(), "acme", "widget", 42)
		if err == nil || !strings.Contains(err.Error(), "list PR reviews") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: "nope"})
		_, err := NewGHClient().ListPRReviews(context.Background(), "acme", "widget", 42)
		if err == nil || !strings.Contains(err.Error(), "parse reviews") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_ListPRComments_MergesBothEndpointsInTimeOrder(t *testing.T) {
	calls := newFakeGH(t,
		ghResponse{
			Prefix: "api repos/acme/widget/pulls/42/comments",
			Stdout: `[{"id":20,"path":"main.go","line":7,"side":"RIGHT","body":"inline note",
				"created_at":"2026-01-05T12:00:00Z","updated_at":"2026-01-05T12:30:00Z",
				"in_reply_to_id":19,"html_url":"https://github.com/acme/widget/pull/42#discussion_r20",
				"user":{"login":"alice","avatar_url":"https://a","type":"User"}}]`,
		},
		ghResponse{
			Prefix: "api repos/acme/widget/issues/42/comments",
			Stdout: `[{"id":10,"body":"conversation note","created_at":"2026-01-05T09:00:00Z",
				"updated_at":"2026-01-05T09:00:00Z",
				"html_url":"https://github.com/acme/widget/pull/42#issuecomment-10",
				"user":{"login":"dependabot","avatar_url":"https://d","type":"Bot"}}]`,
		},
	)

	comments, err := NewGHClient().ListPRComments(context.Background(), "acme", "widget", 42, nil)
	if err != nil {
		t.Fatalf("ListPRComments: %v", err)
	}
	recorded := calls(t)
	assertGHArgv(t, recorded, 0, []string{"api", "repos/acme/widget/pulls/42/comments", "--paginate"})
	assertGHArgv(t, recorded, 1, []string{"api", "repos/acme/widget/issues/42/comments", "--paginate"})
	if len(comments) != 2 {
		t.Fatalf("comments = %#v, want 2", comments)
	}
	if comments[0].ID != 10 || comments[0].CommentType != commentTypeIssue || !comments[0].AuthorIsBot {
		t.Errorf("comments[0] = %#v, want the older bot-authored issue comment first", comments[0])
	}
	assertPRCommentHTMLURL(t, comments[0], "https://github.com/acme/widget/pull/42#issuecomment-10")
	second := comments[1]
	if second.ID != 20 || second.CommentType != commentTypeReview || second.AuthorIsBot {
		t.Errorf("comments[1] = %#v", second)
	}
	if second.Path != "main.go" || second.Line != 7 || second.Side != "RIGHT" {
		t.Errorf("comments[1] location = %#v", second)
	}
	if second.InReplyTo == nil || *second.InReplyTo != 19 {
		t.Errorf("in_reply_to = %v, want 19", second.InReplyTo)
	}
	assertPRCommentHTMLURL(t, second, "https://github.com/acme/widget/pull/42#discussion_r20")
}

func TestGHClient_ListPRComments_AppendsSinceToBothEndpoints(t *testing.T) {
	calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: `[]`})
	since := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := NewGHClient().ListPRComments(context.Background(), "acme", "widget", 42, &since); err != nil {
		t.Fatalf("ListPRComments: %v", err)
	}
	recorded := calls(t)
	if len(recorded) != 2 {
		t.Fatalf("gh calls = %d, want 2", len(recorded))
	}
	wantSuffix := "?since=" + "2026-01-02T03%3A04%3A05Z"
	for i, argv := range recorded {
		if !strings.HasSuffix(argv[1], wantSuffix) {
			t.Errorf("argv[%d] endpoint = %q, want the escaped since suffix %q", i, argv[1], wantSuffix)
		}
	}
}

func TestGHClient_ListPRComments_Errors(t *testing.T) {
	t.Run("review leg failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/acme/widget/pulls/", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().ListPRComments(context.Background(), "acme", "widget", 42, nil)
		if err == nil || !strings.Contains(err.Error(), "list PR comments") {
			t.Fatalf("err = %v, want a wrapped review-comments error", err)
		}
	})
	t.Run("issue leg failure", func(t *testing.T) {
		newFakeGH(t,
			ghResponse{Prefix: "api repos/acme/widget/pulls/", Stdout: `[]`},
			ghResponse{Prefix: "api repos/acme/widget/issues/", Stderr: "boom", Exit: 1},
		)
		_, err := NewGHClient().ListPRComments(context.Background(), "acme", "widget", 42, nil)
		if err == nil || !strings.Contains(err.Error(), "list issue comments") {
			t.Fatalf("err = %v, want a wrapped issue-comments error", err)
		}
	})
	t.Run("unparseable review comments", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/acme/widget/pulls/", Stdout: "nope"})
		_, err := NewGHClient().ListPRComments(context.Background(), "acme", "widget", 42, nil)
		if err == nil || !strings.Contains(err.Error(), "parse comments") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
	t.Run("unparseable issue comments", func(t *testing.T) {
		newFakeGH(t,
			ghResponse{Prefix: "api repos/acme/widget/pulls/", Stdout: `[]`},
			ghResponse{Prefix: "api repos/acme/widget/issues/", Stdout: "nope"},
		)
		_, err := NewGHClient().ListPRComments(context.Background(), "acme", "widget", 42, nil)
		if err == nil || !strings.Contains(err.Error(), "parse issue comments") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestAppendSinceQuery(t *testing.T) {
	const endpoint = "repos/o/r/pulls/1/comments"
	if got := appendSinceQuery(endpoint, nil); got != endpoint {
		t.Errorf("appendSinceQuery(nil) = %q, want the endpoint unchanged", got)
	}
	since := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	want := endpoint + "?since=2026-01-02T03%3A04%3A05Z"
	if got := appendSinceQuery(endpoint, &since); got != want {
		t.Errorf("appendSinceQuery = %q, want %q", got, want)
	}
}

// A 404 means "no protection rule" and a 403 means "no rule we can see"
// (the token lacks Administration: Read). Both must be cached as HasRule=false
// without an error so the poller stops re-asking.
func TestGHClient_FetchBranchProtection(t *testing.T) {
	cases := []struct {
		name         string
		resp         ghResponse
		wantHasRule  bool
		wantRequired int
		wantErr      bool
	}{
		{
			name: "rule with a required approval count",
			resp: ghResponse{
				Prefix: "api repos/",
				Stdout: `{"required_pull_request_reviews":{"required_approving_review_count":2}}`,
			},
			wantHasRule: true, wantRequired: 2,
		},
		{
			name:        "rule without a review requirement",
			resp:        ghResponse{Prefix: "api repos/", Stdout: `{"required_pull_request_reviews":null}`},
			wantHasRule: true,
		},
		{
			name: "404 maps to no rule",
			resp: ghResponse{Prefix: "api repos/", Stderr: "gh: HTTP 404: Not Found", Exit: 1},
		},
		{
			name: "403 maps to no rule",
			resp: ghResponse{Prefix: "api repos/", Stderr: "gh: HTTP 403: Forbidden", Exit: 1},
		},
		{
			name:    "500 propagates",
			resp:    ghResponse{Prefix: "api repos/", Stderr: "gh: HTTP 500: server error", Exit: 1},
			wantErr: true,
		},
		{
			name:    "unparseable payload propagates",
			resp:    ghResponse{Prefix: "api repos/", Stdout: "nope"},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := newFakeGH(t, tc.resp)
			got, err := NewGHClient().FetchBranchProtection(context.Background(), "acme", "widget", "main")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("err = nil, want an error; protection = %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FetchBranchProtection: %v", err)
			}
			assertGHArgv(t, calls(t), 0, []string{
				"api", "repos/acme/widget/branches/main/protection",
			})
			if got.HasRule != tc.wantHasRule {
				t.Errorf("HasRule = %v, want %v", got.HasRule, tc.wantHasRule)
			}
			if got.RequiredApprovingReviewCount != tc.wantRequired {
				t.Errorf("required count = %d, want %d",
					got.RequiredApprovingReviewCount, tc.wantRequired)
			}
		})
	}
}

func TestGHClient_ListPRFiles(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api repos/",
		Stdout: `[
			{"filename":"main.go","status":"modified","additions":4,"deletions":1,
			 "patch":"@@ -1 +1 @@\n-old\n+new"},
			{"filename":"new.go","previous_filename":"old.go","status":"renamed"}
		]`,
	})

	files, err := NewGHClient().ListPRFiles(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("ListPRFiles: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{"api", "repos/acme/widget/pulls/42/files", "--paginate"})
	if len(files) != 2 {
		t.Fatalf("files = %#v, want 2", files)
	}
	if files[0].Filename != "main.go" || files[0].Additions != 4 || files[0].Deletions != 1 {
		t.Errorf("files[0] = %#v", files[0])
	}
	if !strings.Contains(files[0].Patch, "+new") {
		t.Errorf("files[0] patch = %q, want the hunk preserved", files[0].Patch)
	}
	if files[1].OldPath != "old.go" || files[1].Status != "renamed" {
		t.Errorf("files[1] = %#v, want the rename source preserved", files[1])
	}
}

func TestGHClient_ListPRFiles_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "boom", Exit: 1})
		files, err := NewGHClient().ListPRFiles(context.Background(), "acme", "widget", 42)
		if files != nil {
			t.Errorf("files = %#v, want nil", files)
		}
		if err == nil || !strings.Contains(err.Error(), "list PR files") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: "nope"})
		_, err := NewGHClient().ListPRFiles(context.Background(), "acme", "widget", 42)
		if err == nil || !strings.Contains(err.Error(), "parse PR files") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_ListPRCommits(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api repos/",
		Stdout: `[
			{"sha":"aaa","commit":{"message":"feat: add thing\n\nlong body",
			 "author":{"date":"2026-01-07T10:00:00Z"}},"author":{"login":"alice"}},
			{"sha":"bbb","commit":{"message":"fix: typo","author":{"date":"2026-01-08T10:00:00Z"}}}
		]`,
	})

	commits, err := NewGHClient().ListPRCommits(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("ListPRCommits: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{"api", "repos/acme/widget/pulls/42/commits", "--paginate"})
	if len(commits) != 2 {
		t.Fatalf("commits = %#v, want 2", commits)
	}
	if commits[0].SHA != "aaa" || commits[0].AuthorLogin != "alice" {
		t.Errorf("commits[0] = %#v", commits[0])
	}
	if commits[0].Message != "feat: add thing" {
		t.Errorf("commits[0] message = %q, want only the subject line", commits[0].Message)
	}
	if commits[0].AuthorDate != "2026-01-07T10:00:00Z" || commits[0].StatsAvailable {
		t.Errorf("commits[0] = %#v, want the raw date and no stats", commits[0])
	}
	if commits[1].AuthorLogin != "" {
		t.Errorf("commits[1] author = %q, want empty for an unmatched account", commits[1].AuthorLogin)
	}
}

func TestGHClient_ListPRCommits_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().ListPRCommits(context.Background(), "acme", "widget", 42)
		if err == nil || !strings.Contains(err.Error(), "list PR commits") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: "nope"})
		_, err := NewGHClient().ListPRCommits(context.Background(), "acme", "widget", 42)
		if err == nil || !strings.Contains(err.Error(), "parse PR commits") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_GetPRCommitDetail_RejectsANonSHA(t *testing.T) {
	calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: `[]`})
	_, err := NewGHClient().GetPRCommitDetail(context.Background(), "acme", "widget", "not-a-sha")
	if err == nil || !strings.Contains(err.Error(), "invalid commit SHA") {
		t.Fatalf("err = %v, want the SHA validation to reject it", err)
	}
	if len(calls(t)) != 0 {
		t.Errorf("gh calls = %d, want 0 — validation must run before exec", len(calls(t)))
	}
}

func TestGHClient_SubmitReview(t *testing.T) {
	t.Run("approve without a body omits the body field", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: `{}`})
		if err := NewGHClient().SubmitReview(
			context.Background(), "acme", "widget", 42, "APPROVE", "",
		); err != nil {
			t.Fatalf("SubmitReview: %v", err)
		}
		assertGHArgv(t, calls(t), 0, []string{
			"api", "repos/acme/widget/pulls/42/reviews", "-X", "POST", "-f", "event=APPROVE",
		})
	})
	t.Run("request changes carries the body", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: `{}`})
		if err := NewGHClient().SubmitReview(
			context.Background(), "acme", "widget", 42, "REQUEST_CHANGES", "please fix",
		); err != nil {
			t.Fatalf("SubmitReview: %v", err)
		}
		assertGHArgv(t, calls(t), 0, []string{
			"api", "repos/acme/widget/pulls/42/reviews", "-X", "POST",
			"-f", "event=REQUEST_CHANGES", "-f", "body=please fix",
		})
	})
	t.Run("failure names the PR", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "HTTP 422", Exit: 1})
		err := NewGHClient().SubmitReview(context.Background(), "acme", "widget", 42, "APPROVE", "")
		if err == nil || !strings.Contains(err.Error(), "submit review on PR #42") {
			t.Fatalf("err = %v, want the PR named in the wrap", err)
		}
	})
}

func TestGHClient_MergePR(t *testing.T) {
	t.Run("sends the merge method", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: `{"status":"merged"}`})
		if _, err := NewGHClient().MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"}); err != nil {
			t.Fatalf("MergePR: %v", err)
		}
		assertGHArgv(t, calls(t), 0, []string{
			"api", "repos/acme/widget/pulls/42/merge-async", "-X", "PUT",
			"-f", "merge_action=default", "-f", "merge_method=squash",
		})
	})
	t.Run("omits the merge method when unset", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: `{"status":"merged"}`})
		if _, err := NewGHClient().MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{}); err != nil {
			t.Fatalf("MergePR: %v", err)
		}
		assertGHArgv(t, calls(t), 0, []string{
			"api", "repos/acme/widget/pulls/42/merge-async", "-X", "PUT",
			"-f", "merge_action=default",
		})
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.1
func TestGHClient_MergePR_SendsExpectedHeadSHA(t *testing.T) {
	calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: `{"status":"merged"}`})
	request := MergePRRequest{MergeMethod: "squash", ExpectedHeadSHA: "abc123"}
	if _, err := NewGHClient().MergePR(context.Background(), "acme", "widget", 42, request); err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "repos/acme/widget/pulls/42/merge-async", "-X", "PUT",
		"-f", "merge_action=default", "-f", "merge_method=squash", "-f", "sha=abc123",
	})
}

// The gh path must surface merge rejections as *GitHubAPIError so httpMergePR
// can translate them the same way it does for the PAT client.
func TestGHClient_MergePR_StatusIsRecoverable(t *testing.T) {
	cases := []struct {
		stderr string
		want   int
	}{
		{"gh: HTTP 405: Method Not Allowed", http.StatusMethodNotAllowed},
		{"gh: HTTP 404: Not Found", http.StatusNotFound},
		{"gh: HTTP 403: Forbidden", http.StatusForbidden},
		{"gh: merge conflict (HTTP 409)", http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.want), func(t *testing.T) {
			newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: tc.stderr, Exit: 1})
			_, err := NewGHClient().MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "merge"})
			var apiErr *GitHubAPIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want a *GitHubAPIError", err)
			}
			if apiErr.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", apiErr.StatusCode, tc.want)
			}
			if apiErr.Endpoint != "repos/acme/widget/pulls/42/merge-async" {
				t.Errorf("endpoint = %q", apiErr.Endpoint)
			}
		})
	}
}

func TestGHClient_MergePR_AlreadyQueuedIsIdempotent(t *testing.T) {
	newFakeGH(t,
		ghResponse{Prefix: "api repos/acme/widget/pulls/42/merge-async/request-1", Stdout: `{"status":"enqueued","details":{"message":"Pull request was added to the merge queue."}}`},
		ghResponse{Prefix: "api repos/", Stderr: `{"status":"pending","details":{"message":"Merge request enqueued.","uuid":"request-1","merge_method":"squash","merge_action":"default","expected_head_sha":"abc123"}} gh: HTTP 409`, Exit: 1},
	)
	outcome, err := NewGHClient().MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"})
	if err != nil || outcome != MergeOutcomeQueued {
		t.Fatalf("outcome = %q, err = %v, want queued", outcome, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.3
func TestGHClient_MergePR_PollsNestedPendingResponse(t *testing.T) {
	calls := newFakeGH(t,
		ghResponse{Prefix: "api repos/acme/widget/pulls/42/merge-async/request-1", Stdout: `{"status":"merged","details":{"sha":"abc123"}}`},
		ghResponse{Prefix: "api repos/", Stdout: `{"status":"pending","details":{"message":"Merge request enqueued.","uuid":"request-1","merge_method":"squash","merge_action":"default","expected_head_sha":"abc123"}}`},
	)

	outcome, err := NewGHClient().MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"})
	if err != nil || outcome != MergeOutcomeMerged {
		t.Fatalf("outcome = %q, err = %v, want merged", outcome, err)
	}
	assertGHArgv(t, calls(t), 1, []string{
		"api", "repos/acme/widget/pulls/42/merge-async/request-1",
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.7
func TestGHClient_MergePR_PendingPollHonorsContextBeforeNextRequest(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api repos/",
		Stdout: `{"status":"pending","details":{"uuid":"request-1"}}`,
	})
	client := NewGHClient()
	var gotDelay time.Duration
	client.mergePollWait = func(_ context.Context, delay time.Duration) error {
		gotDelay = delay
		return context.DeadlineExceeded
	}

	_, err := client.MergePR(
		context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("MergePR error = %v, want context deadline", err)
	}
	if got := len(calls(t)); got != 1 {
		t.Fatalf("request count = %d, want only the initial request before cancellation", got)
	}
	if gotDelay != time.Second {
		t.Fatalf("poll delay = %s, want one second", gotDelay)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.13
func TestGHClient_MergePR_ReportsNestedFailureMessage(t *testing.T) {
	newFakeGH(t, ghResponse{
		Prefix: "api repos/",
		Stdout: `{"status":"failed","details":{"message":"Branch protection blocked the merge.","expected_head_sha":"abc123"}}`,
	})

	_, err := NewGHClient().MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "squash"})
	if err == nil || !strings.Contains(err.Error(), "Branch protection blocked the merge.") ||
		!strings.Contains(err.Error(), "abc123") {
		t.Fatalf("err = %v, want the nested provider message and expected head", err)
	}
}

func TestGHClient_MergePR_UnmappedStatusFallsBackToAPlainWrap(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "gh: HTTP 500: server error", Exit: 1})
	_, err := NewGHClient().MergePR(context.Background(), "acme", "widget", 42, MergePRRequest{MergeMethod: "merge"})
	if err == nil || !strings.Contains(err.Error(), "merge PR #42") {
		t.Fatalf("err = %v, want a plain wrap naming the PR", err)
	}
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) {
		t.Errorf("a 500 must not become *GitHubAPIError, got %#v", apiErr)
	}
}

func TestGHClient_GetRepoMergeMethods(t *testing.T) {
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
			"rebase only",
			`{"allow_merge_commit":false,"allow_squash_merge":false,"allow_rebase_merge":true}`,
			RepoMergeMethods{Rebase: true},
		},
		{"omitted fields read as disallowed", `{"full_name":"acme/widget"}`, RepoMergeMethods{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: tc.body})
			got, err := NewGHClient().GetRepoMergeMethods(context.Background(), "acme", "widget")
			if err != nil {
				t.Fatalf("GetRepoMergeMethods: %v", err)
			}
			assertGHArgv(t, calls(t), 0, []string{"api", "repos/acme/widget"})
			if got != tc.want {
				t.Errorf("methods = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestGHClient_GetRepoMergeMethods_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().GetRepoMergeMethods(context.Background(), "acme", "widget")
		if err == nil || !strings.Contains(err.Error(), "get repo merge methods") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: "nope"})
		_, err := NewGHClient().GetRepoMergeMethods(context.Background(), "acme", "widget")
		if err == nil || !strings.Contains(err.Error(), "parse repo") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

// ListRepoBranches reads a --jq-projected newline stream rather than JSON, so
// blank lines and stray whitespace must be dropped.
func TestGHClient_ListRepoBranches(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api repos/",
		Stdout: "main\n  develop  \n\nrelease/1.0\n\n",
	})
	branches, err := NewGHClient().ListRepoBranches(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("ListRepoBranches: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "repos/acme/widget/branches", "-X", "GET",
		"-f", "per_page=100", "--paginate", "--jq", ".[].name",
	})
	want := []string{"main", "develop", "release/1.0"}
	if len(branches) != len(want) {
		t.Fatalf("branches = %#v, want %v", branches, want)
	}
	for i, name := range want {
		if branches[i].Name != name {
			t.Errorf("branches[%d] = %q, want %q", i, branches[i].Name, name)
		}
	}
}

func TestGHClient_ListRepoBranches_Error(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "boom", Exit: 1})
	branches, err := NewGHClient().ListRepoBranches(context.Background(), "acme", "widget")
	if branches != nil {
		t.Errorf("branches = %#v, want nil", branches)
	}
	if err == nil || !strings.Contains(err.Error(), "list repo branches") {
		t.Fatalf("err = %v, want a wrapped error", err)
	}
}

func TestGHClient_CreateGist_PipesThePayloadOnStdin(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api gists",
		Stdout: `{"id":"abc123","html_url":"https://gist.github.com/abc123",
			"created_at":"2026-07-01T12:00:00Z"}`,
	})

	resp, err := NewGHClient().CreateGist(context.Background(), CreateGistInput{
		Description: "kandev share",
		Public:      false,
		Files:       map[string]GistFile{"README.md": {Content: "# hello"}},
	})
	if err != nil {
		t.Fatalf("CreateGist: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "gists", "-X", "POST", "-H", "Accept: " + githubAccept, "--input", "-",
	})
	if resp.ID != "abc123" || resp.HTMLURL != "https://gist.github.com/abc123" {
		t.Errorf("response = %#v", resp)
	}
	if !resp.CreatedAt.Equal(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("created_at = %v", resp.CreatedAt)
	}
	var payload struct {
		Description string                       `json:"description"`
		Public      bool                         `json:"public"`
		Files       map[string]map[string]string `json:"files"`
	}
	stdin := ghStdin(t)
	if err := json.Unmarshal([]byte(stdin), &payload); err != nil {
		t.Fatalf("decode piped payload %q: %v", stdin, err)
	}
	if payload.Description != "kandev share" || payload.Public {
		t.Errorf("payload meta = %#v", payload)
	}
	if payload.Files["README.md"]["content"] != "# hello" {
		t.Errorf("payload files = %#v", payload.Files)
	}
}

func TestGHClient_CreateGist_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api gists", Stderr: "HTTP 401", Exit: 1})
		resp, err := NewGHClient().CreateGist(context.Background(), CreateGistInput{})
		if resp != nil {
			t.Errorf("resp = %#v, want nil", resp)
		}
		if err == nil || !strings.Contains(err.Error(), "create gist") {
			t.Fatalf("err = %v, want a wrapped create-gist error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api gists", Stdout: "nope"})
		_, err := NewGHClient().CreateGist(context.Background(), CreateGistInput{})
		if err == nil || !strings.Contains(err.Error(), "decode gist response") {
			t.Fatalf("err = %v, want a decode failure", err)
		}
	})
}

func TestGHClient_DeleteGist(t *testing.T) {
	t.Run("deletes by id", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "api gists/", Stdout: ""})
		if err := NewGHClient().DeleteGist(context.Background(), "abc123"); err != nil {
			t.Fatalf("DeleteGist: %v", err)
		}
		assertGHArgv(t, calls(t), 0, []string{"api", "gists/abc123", "-X", "DELETE"})
	})
	t.Run("empty id never execs gh", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "api gists/", Stdout: ""})
		if err := NewGHClient().DeleteGist(context.Background(), ""); err == nil {
			t.Fatal("expected an error for an empty gist id")
		}
		if len(calls(t)) != 0 {
			t.Errorf("gh calls = %d, want 0", len(calls(t)))
		}
	})
	// share.IsAlreadyGone matches the typed 404 to treat an already-revoked
	// gist as a soft success — every gh 404 spelling must be recognised.
	t.Run("404 spellings all become a typed error", func(t *testing.T) {
		for _, stderr := range []string{"HTTP 404: Not Found", "404 Not Found", "status: 404"} {
			t.Run(stderr, func(t *testing.T) {
				newFakeGH(t, ghResponse{Prefix: "api gists/", Stderr: stderr, Exit: 1})
				err := NewGHClient().DeleteGist(context.Background(), "gone")
				var apiErr *GitHubAPIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
					t.Fatalf("err = %v, want a *GitHubAPIError with 404", err)
				}
				if apiErr.Endpoint != "/gists/gone" {
					t.Errorf("endpoint = %q, want /gists/gone", apiErr.Endpoint)
				}
			})
		}
	})
	t.Run("other failures stay untyped", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api gists/", Stderr: "HTTP 500", Exit: 1})
		err := NewGHClient().DeleteGist(context.Background(), "abc123")
		if err == nil || !strings.Contains(err.Error(), "delete gist abc123") {
			t.Fatalf("err = %v, want a wrapped delete-gist error", err)
		}
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			t.Errorf("a 500 must not become *GitHubAPIError, got %#v", apiErr)
		}
	})
}
