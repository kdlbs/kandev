package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Compile-time interface check.
var _ Client = (*MockClient)(nil)

func TestMockClient_IsAuthenticated(t *testing.T) {
	m := NewMockClient()
	ok, err := m.IsAuthenticated(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected authenticated=true")
	}
}

func TestMockClient_GetAuthenticatedUser(t *testing.T) {
	m := NewMockClient()
	user, err := m.GetAuthenticatedUser(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != mockDefaultUser {
		t.Fatalf("expected mock-user, got %q", user)
	}
}

func TestMockClient_SetUser(t *testing.T) {
	m := NewMockClient()
	m.SetUser("custom-user")
	user, _ := m.GetAuthenticatedUser(context.Background())
	if user != "custom-user" {
		t.Fatalf("expected custom-user, got %q", user)
	}
}

func TestMockClient_AddPR_GetPR(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	// Not found
	_, err := m.GetPR(ctx, "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error for missing PR")
	}

	// Add and retrieve
	m.AddPR(&PR{
		Number:     1,
		Title:      "Test PR",
		RepoOwner:  "owner",
		RepoName:   "repo",
		HeadBranch: "feature",
	})

	pr, err := m.GetPR(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Title != "Test PR" {
		t.Fatalf("expected title 'Test PR', got %q", pr.Title)
	}
}

func TestMockClient_AddIssueGetIssueAndReset(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	_, err := m.GetIssue(ctx, "owner", "repo", 1456)
	if err == nil || err.Error() != "mock: issue owner/repo#1456 not found" {
		t.Fatalf("expected missing issue error, got %v", err)
	}

	m.AddIssue(&Issue{
		Number:    1456,
		Title:     "Test issue",
		RepoOwner: "owner",
		RepoName:  "repo",
	})

	issue, err := m.GetIssue(ctx, "owner", "repo", 1456)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Title != "Test issue" {
		t.Fatalf("expected title 'Test issue', got %q", issue.Title)
	}

	m.Reset()
	_, err = m.GetIssue(ctx, "owner", "repo", 1456)
	if err == nil || err.Error() != "mock: issue owner/repo#1456 not found" {
		t.Fatalf("expected reset issue error, got %v", err)
	}
}

func TestMockClient_ListIssuesPaged(t *testing.T) {
	m := NewMockClient()
	m.AddIssue(&Issue{Number: 1, RepoOwner: "owner", RepoName: "repo"})
	m.AddIssue(&Issue{Number: 2, RepoOwner: "owner", RepoName: "repo"})

	page, err := m.ListIssuesPaged(context.Background(), "state:open", "", 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Issues) != 2 || page.TotalCount != 2 {
		t.Fatalf("expected two issues, got %#v", page)
	}
	if page.Page != 2 || page.PerPage != 10 {
		t.Fatalf("expected pagination metadata to be preserved, got %#v", page)
	}
}

func TestMockClient_FindPRByBranch(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	// Not found returns nil, nil
	pr, err := m.FindPRByBranch(ctx, "owner", "repo", "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr != nil {
		t.Fatal("expected nil PR for missing branch")
	}

	// Add and find
	m.AddPR(&PR{
		Number:     1,
		RepoOwner:  "owner",
		RepoName:   "repo",
		HeadBranch: "feature",
		Title:      "Branch PR",
	})

	pr, err = m.FindPRByBranch(ctx, "owner", "repo", "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr == nil || pr.Title != "Branch PR" {
		t.Fatalf("expected Branch PR, got %v", pr)
	}
}

func TestMockClient_ListAuthoredPRs(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	m.AddPR(&PR{Number: 1, RepoOwner: "o", RepoName: "r", AuthorLogin: mockDefaultUser})
	m.AddPR(&PR{Number: 2, RepoOwner: "o", RepoName: "r", AuthorLogin: "other"})
	m.AddPR(&PR{Number: 3, RepoOwner: "x", RepoName: "y", AuthorLogin: mockDefaultUser})

	prs, err := m.ListAuthoredPRs(ctx, "o", "r")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 authored PR, got %d", len(prs))
	}
	if prs[0].Number != 1 {
		t.Fatalf("expected PR #1, got #%d", prs[0].Number)
	}
}

func TestMockClient_ListReviewRequestedPRs(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	m.AddPR(&PR{Number: 1, RepoOwner: "o", RepoName: "r"})
	m.AddPR(&PR{
		Number:             2,
		RepoOwner:          "o",
		RepoName:           "r",
		RequestedReviewers: []RequestedReviewer{{Login: "user1", Type: "user"}},
	})

	prs, err := m.ListReviewRequestedPRs(ctx, "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 review-requested PR, got %d", len(prs))
	}
	if prs[0].Number != 2 {
		t.Fatalf("expected PR #2, got #%d", prs[0].Number)
	}
}

func TestMockClient_ListPRComments_Since(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	now := time.Now()
	old := now.Add(-1 * time.Hour)
	recent := now.Add(1 * time.Hour)

	m.AddComments("o", "r", 1, []PRComment{
		{ID: 1, UpdatedAt: old},
		{ID: 2, UpdatedAt: recent},
	})

	// Without since — all
	all, err := m.ListPRComments(ctx, "o", "r", 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(all))
	}

	// With since — only recent
	filtered, err := m.ListPRComments(ctx, "o", "r", 1, &now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != 2 {
		t.Fatalf("expected 1 filtered comment (ID=2), got %v", filtered)
	}
}

func TestMockClient_SubmitReview(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	err := m.SubmitReview(ctx, "o", "r", 1, "APPROVE", "LGTM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviews := m.SubmittedReviews()
	if len(reviews) != 1 {
		t.Fatalf("expected 1 submitted review, got %d", len(reviews))
	}
	if reviews[0].Event != "APPROVE" || reviews[0].Body != "LGTM" {
		t.Fatalf("unexpected review: %+v", reviews[0])
	}
}

func TestMockClient_CreateAndDeleteGist(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	resp, err := m.CreateGist(ctx, CreateGistInput{
		Description: "snapshot",
		Public:      false,
		Files: map[string]GistFile{
			"snapshot.json": {Content: `{"x":1}`},
			"README.md":     {Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("CreateGist error: %v", err)
	}
	if resp.ID == "" || resp.HTMLURL == "" {
		t.Fatalf("expected non-empty id/url, got %+v", resp)
	}

	gists := m.Gists()
	g, ok := gists[resp.ID]
	if !ok {
		t.Fatalf("gist %s not stored", resp.ID)
	}
	if g.Public {
		t.Fatal("gist should default to secret (Public=false)")
	}
	if _, ok := g.Files["snapshot.json"]; !ok {
		t.Fatal("snapshot.json missing from stored gist")
	}

	if err := m.DeleteGist(ctx, resp.ID); err != nil {
		t.Fatalf("DeleteGist error: %v", err)
	}
	if _, ok := m.Gists()[resp.ID]; ok {
		t.Fatal("gist still present after delete")
	}
	if got := m.DeletedGists(); len(got) != 1 || got[0] != resp.ID {
		t.Fatalf("expected DeletedGists=[%q], got %v", resp.ID, got)
	}

	err = m.DeleteGist(ctx, resp.ID)
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("expected 404 GitHubAPIError on second delete, got %v", err)
	}
}

func TestMockClient_Reset(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	m.SetUser("custom")
	m.AddPR(&PR{Number: 1, RepoOwner: "o", RepoName: "r", HeadBranch: "b"})
	m.AddOrgs([]GitHubOrg{{Login: "org1"}})

	m.Reset()

	user, _ := m.GetAuthenticatedUser(ctx)
	if user != mockDefaultUser {
		t.Fatalf("expected user reset to mock-user, got %q", user)
	}

	_, err := m.GetPR(ctx, "o", "r", 1)
	if err == nil {
		t.Fatal("expected error after reset")
	}

	orgs, _ := m.ListUserOrgs(ctx)
	if len(orgs) != 0 {
		t.Fatalf("expected 0 orgs after reset, got %d", len(orgs))
	}
}
