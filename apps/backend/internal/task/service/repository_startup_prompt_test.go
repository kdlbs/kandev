package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestService_ResolveRepositoryStartupPrompt covers the four ways the
// helper can be called from the MCP handler and the watcher adapter.
func TestService_ResolveRepositoryStartupPrompt(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	ws, err := svc.CreateWorkspace(ctx, &CreateWorkspaceRequest{Name: "sp-workspace", OwnerID: "u"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	with := &models.Repository{
		ID:            "repo-with-prompt",
		WorkspaceID:   ws.ID,
		Name:          "with-prompt",
		SourceType:    "local",
		StartupPrompt: "Read {{TICKET_URL}} carefully.\nThen begin work on {{TASK_TITLE}}.",
	}
	if err := repo.CreateRepository(ctx, with); err != nil {
		t.Fatalf("create with-prompt repo: %v", err)
	}
	without := &models.Repository{
		ID:          "repo-without-prompt",
		WorkspaceID: ws.ID,
		Name:        "no-prompt",
		SourceType:  "local",
	}
	if err := repo.CreateRepository(ctx, without); err != nil {
		t.Fatalf("create no-prompt repo: %v", err)
	}

	t.Run("empty repo ID returns empty", func(t *testing.T) {
		got := svc.ResolveRepositoryStartupPrompt(ctx, "", "Fix", nil)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("repository without startup_prompt returns empty", func(t *testing.T) {
		got := svc.ResolveRepositoryStartupPrompt(ctx, without.ID, "Fix", nil)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("missing repository returns empty (not an error)", func(t *testing.T) {
		got := svc.ResolveRepositoryStartupPrompt(ctx, "does-not-exist", "Fix", nil)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("watcher-imported task with jira metadata resolves both lines", func(t *testing.T) {
		metadata := map[string]interface{}{
			"jira_issue_key": "PROJ-42",
			"jira_issue_url": "https://x.atlassian.net/browse/PROJ-42",
		}
		got := svc.ResolveRepositoryStartupPrompt(ctx, with.ID, "Fix billing", metadata)
		want := "Read https://x.atlassian.net/browse/PROJ-42 carefully.\nThen begin work on Fix billing."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("manual task without ticket metadata drops ticket line", func(t *testing.T) {
		got := svc.ResolveRepositoryStartupPrompt(ctx, with.ID, "Refactor billing", nil)
		want := "Then begin work on Refactor billing."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// TestService_FindRepositoryForInput_WorkspaceOwnership pins that
// FindRepositoryForInput enforces workspace ownership on the RepositoryID
// branch: a caller from workspace A that supplies a repository_id from
// workspace B receives nil, not the foreign repo. Prevents a
// cross-workspace startup_prompt leak via the MCP path.
func TestService_FindRepositoryForInput_WorkspaceOwnership(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsA, err := svc.CreateWorkspace(ctx, &CreateWorkspaceRequest{Name: "ws-a", OwnerID: "u"})
	if err != nil {
		t.Fatalf("create workspace A: %v", err)
	}
	wsB, err := svc.CreateWorkspace(ctx, &CreateWorkspaceRequest{Name: "ws-b", OwnerID: "u"})
	if err != nil {
		t.Fatalf("create workspace B: %v", err)
	}
	repoB := &models.Repository{
		ID:            "repo-in-ws-b",
		WorkspaceID:   wsB.ID,
		Name:          "foreign",
		SourceType:    "local",
		StartupPrompt: "leaked prompt",
	}
	if err := repo.CreateRepository(ctx, repoB); err != nil {
		t.Fatalf("create workspace-B repo: %v", err)
	}

	// Caller says "I'm in workspace A", but supplies workspace B's repo id.
	got, err := svc.FindRepositoryForInput(ctx, wsA.ID, TaskRepositoryInput{RepositoryID: repoB.ID})
	if err != nil {
		t.Fatalf("FindRepositoryForInput: %v", err)
	}
	if got != nil {
		t.Errorf("cross-workspace lookup returned repo %+v, want nil", got)
	}

	// Same repo, correct workspace, does resolve.
	got, err = svc.FindRepositoryForInput(ctx, wsB.ID, TaskRepositoryInput{RepositoryID: repoB.ID})
	if err != nil {
		t.Fatalf("FindRepositoryForInput (own ws): %v", err)
	}
	if got == nil || got.ID != repoB.ID {
		t.Errorf("same-workspace lookup got %+v, want repo id %s", got, repoB.ID)
	}

	// Empty workspaceID is treated as "no scope" (used by legacy callers).
	got, err = svc.FindRepositoryForInput(ctx, "", TaskRepositoryInput{RepositoryID: repoB.ID})
	if err != nil {
		t.Fatalf("FindRepositoryForInput (no scope): %v", err)
	}
	if got == nil || got.ID != repoB.ID {
		t.Errorf("no-scope lookup got %+v, want repo id %s", got, repoB.ID)
	}
}
