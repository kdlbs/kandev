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
