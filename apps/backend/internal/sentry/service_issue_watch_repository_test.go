package sentry

import (
	"context"
	"errors"
	"testing"
)

// fakeRepoLookup is a static RepositoryLookup for repository-binding tests.
type fakeRepoLookup struct {
	workspaceID   string
	defaultBranch string
	ok            bool
}

func (f fakeRepoLookup) GetRepository(_ context.Context, _ string) (string, string, bool) {
	return f.workspaceID, f.defaultBranch, f.ok
}

func TestService_CreateIssueWatch_RepositoryBinding(t *testing.T) {
	ctx := context.Background()
	baseReq := func() *CreateIssueWatchRequest {
		return &CreateIssueWatchRequest{
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf",
			WorkflowStepID: "step",
			Filter:         SearchFilter{OrgSlug: "acme", ProjectSlug: "frontend"},
		}
	}

	t.Run("default branch filled from repo", func(t *testing.T) {
		f := newSvcFixture(t)
		f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})
		req := baseReq()
		req.RepositoryID = "repo-1"
		w, err := f.svc.CreateIssueWatch(ctx, req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if w.RepositoryID != "repo-1" || w.BaseBranch != "main" {
			t.Fatalf("expected repo-1@main, got repo=%q branch=%q", w.RepositoryID, w.BaseBranch)
		}
	})

	t.Run("explicit branch preserved", func(t *testing.T) {
		f := newSvcFixture(t)
		f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})
		req := baseReq()
		req.RepositoryID = "repo-1"
		req.BaseBranch = "release/v2"
		w, err := f.svc.CreateIssueWatch(ctx, req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if w.BaseBranch != "release/v2" {
			t.Fatalf("explicit branch overwritten: %q", w.BaseBranch)
		}
	})

	t.Run("cross-workspace repo rejected", func(t *testing.T) {
		f := newSvcFixture(t)
		f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-other", defaultBranch: "main", ok: true})
		req := baseReq()
		req.RepositoryID = "repo-1"
		if _, err := f.svc.CreateIssueWatch(ctx, req); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for cross-workspace repo, got %v", err)
		}
	})

	t.Run("missing repo rejected", func(t *testing.T) {
		f := newSvcFixture(t)
		f.svc.SetRepositoryLookup(fakeRepoLookup{ok: false})
		req := baseReq()
		req.RepositoryID = "ghost"
		if _, err := f.svc.CreateIssueWatch(ctx, req); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for missing repo, got %v", err)
		}
	})

	t.Run("unbound skips lookup and clears branch", func(t *testing.T) {
		f := newSvcFixture(t) // no RepositoryLookup wired
		req := baseReq()
		req.BaseBranch = "ignored-without-repo"
		w, err := f.svc.CreateIssueWatch(ctx, req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if w.RepositoryID != "" || w.BaseBranch != "" {
			t.Fatalf("unbound watch must stay empty, got repo=%q branch=%q", w.RepositoryID, w.BaseBranch)
		}
	})
}

func TestService_UpdateIssueWatch_RepositoryBinding(t *testing.T) {
	ctx := context.Background()
	f := newSvcFixture(t)
	f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})

	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		Filter:         SearchFilter{OrgSlug: "acme", ProjectSlug: "frontend"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.RepositoryID != "" {
		t.Fatalf("expected unbound on create, got %q", w.RepositoryID)
	}

	repoID := "repo-1"
	updated, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{RepositoryID: &repoID})
	if err != nil {
		t.Fatalf("update bind: %v", err)
	}
	if updated.RepositoryID != "repo-1" || updated.BaseBranch != "main" {
		t.Fatalf("expected repo-1@main after update, got repo=%q branch=%q", updated.RepositoryID, updated.BaseBranch)
	}

	empty := ""
	cleared, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{RepositoryID: &empty})
	if err != nil {
		t.Fatalf("update unbind: %v", err)
	}
	if cleared.RepositoryID != "" || cleared.BaseBranch != "" {
		t.Fatalf("expected cleared after update, got repo=%q branch=%q", cleared.RepositoryID, cleared.BaseBranch)
	}
}

func TestService_UpdateIssueWatch_RebindAndDeletedRepo(t *testing.T) {
	ctx := context.Background()
	f := newSvcFixture(t)
	f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})

	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{OrgSlug: "acme", ProjectSlug: "frontend"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	repo1 := "repo-1"
	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{RepositoryID: &repo1}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "develop", ok: true})
	repo2 := "repo-2"
	rebound, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{RepositoryID: &repo2})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if rebound.RepositoryID != "repo-2" || rebound.BaseBranch != "develop" {
		t.Fatalf("rebind should reset branch to new default, got repo=%q branch=%q", rebound.RepositoryID, rebound.BaseBranch)
	}

	f.svc.SetRepositoryLookup(fakeRepoLookup{ok: false})
	prompt := "updated prompt"
	edited, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{Prompt: &prompt})
	if err != nil {
		t.Fatalf("unrelated edit blocked by deleted bound repo: %v", err)
	}
	if edited.Prompt != "updated prompt" || edited.RepositoryID != "repo-2" {
		t.Fatalf("expected prompt updated + binding preserved, got prompt=%q repo=%q", edited.Prompt, edited.RepositoryID)
	}
}
