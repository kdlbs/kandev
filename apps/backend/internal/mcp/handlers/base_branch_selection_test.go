package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestBaseBranchSelection_MCPInputs pins the provenance captured before MCP
// fills a repository default and the inherited source authored by the parent
// and current-task inheritance producers.
func TestBaseBranchSelection_MCPInputs(t *testing.T) {
	h := &Handlers{}

	t.Run("explicit repository default name stays explicit", func(t *testing.T) {
		repos := h.explicitRepoInputsWithDefaults(context.Background(), []mcpRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		})
		if len(repos) != 1 {
			t.Fatalf("repos = %d, want one", len(repos))
		}
		if repos[0].BaseBranchSelectionSource != models.BaseBranchSelectionSourceExplicit {
			t.Fatalf("source = %q, want explicit", repos[0].BaseBranchSelectionSource)
		}
	})

	t.Run("omitted default records automatic intent before filling", func(t *testing.T) {
		repos := h.explicitRepoInputsWithDefaults(context.Background(), []mcpRepositoryInput{
			{RepositoryID: "repo-1"},
		})
		if len(repos) != 1 {
			t.Fatalf("repos = %d, want one", len(repos))
		}
		if repos[0].BaseBranchSelectionSource != models.BaseBranchSelectionSourceRepositoryDefault {
			t.Fatalf("source = %q, want repository default", repos[0].BaseBranchSelectionSource)
		}
	})

	t.Run("local path input keeps automatic intent", func(t *testing.T) {
		repos := h.explicitRepoInputsWithDefaults(context.Background(), []mcpRepositoryInput{
			{LocalPath: "/tmp/widget", BaseBranch: ""},
		})
		if len(repos) != 1 || repos[0].BaseBranchSelectionSource != models.BaseBranchSelectionSourceRepositoryDefault {
			t.Fatalf("repos = %+v, want automatic default intent", repos)
		}
	})

	t.Run("inherited inputs are server authored", func(t *testing.T) {
		repos := inheritedRepoInputs([]*models.TaskRepository{{
			RepositoryID: "repo-1", BaseBranch: "feature/inherited", CheckoutBranch: "feature/dropped",
		}})
		if len(repos) != 1 {
			t.Fatalf("repos = %d, want one", len(repos))
		}
		if repos[0].BaseBranchSelectionSource != models.BaseBranchSelectionSourceInherited {
			t.Fatalf("source = %q, want inherited", repos[0].BaseBranchSelectionSource)
		}
		if repos[0].BaseBranch != "feature/inherited" || repos[0].CheckoutBranch != "" {
			t.Fatalf("inherited input = %+v, want base retained and checkout dropped", repos[0])
		}
	})
}
