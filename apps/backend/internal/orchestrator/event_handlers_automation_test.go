package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// seedAutomationWorkspaceRepos creates a workspace with the given repository
// IDs (each with a distinct default branch derived from its ID) for
// exercising resolveAutomationRepository / resolveExplicitRepositories.
func seedAutomationWorkspaceRepos(t *testing.T, repo *sqliterepo.Repository, workspaceID string, repoIDs []string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for _, id := range repoIDs {
		r := &models.Repository{
			ID:            id,
			WorkspaceID:   workspaceID,
			Name:          id,
			SourceType:    "local",
			LocalPath:     "/tmp/" + id,
			DefaultBranch: "main-" + id,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := repo.CreateRepository(ctx, r); err != nil {
			t.Fatalf("create repository %s: %v", id, err)
		}
	}
}

func TestResolveAutomationRepository_MultipleExplicitRepositories(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-a", "repo-b"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a := &automation.Automation{WorkspaceID: "ws-1", RepositoryIDs: []string{"repo-a", "repo-b"}}
	evt := &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled}

	resolved := svc.resolveAutomationRepository(context.Background(), a, evt)

	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved repositories, got %d: %+v", len(resolved), resolved)
	}
	if resolved[0].RepositoryID != "repo-a" || resolved[0].BaseBranch != "main-repo-a" || resolved[0].CheckoutBranch != "main-repo-a" {
		t.Errorf("unexpected first repository: %+v", resolved[0])
	}
	if resolved[1].RepositoryID != "repo-b" || resolved[1].BaseBranch != "main-repo-b" || resolved[1].CheckoutBranch != "main-repo-b" {
		t.Errorf("unexpected second repository: %+v", resolved[1])
	}
}

func TestResolveAutomationRepository_SkipsUnloadableID(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-a"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a := &automation.Automation{WorkspaceID: "ws-1", RepositoryIDs: []string{"repo-a", "repo-missing"}}
	evt := &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled}

	resolved := svc.resolveAutomationRepository(context.Background(), a, evt)

	if len(resolved) != 1 || resolved[0].RepositoryID != "repo-a" {
		t.Fatalf("expected only repo-a to resolve, got %+v", resolved)
	}
}

func TestResolveAutomationRepository_EmptyListFallsBackToWorkspace(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-only"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a := &automation.Automation{WorkspaceID: "ws-1"}
	evt := &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled}

	resolved := svc.resolveAutomationRepository(context.Background(), a, evt)

	if len(resolved) != 1 || resolved[0].RepositoryID != "repo-only" {
		t.Fatalf("expected fallback to the workspace's only repository, got %+v", resolved)
	}
}
