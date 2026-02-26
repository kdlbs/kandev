package github

import (
	"context"
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
