package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestNormalizeWorkspaceSourcesForCreationMakesExplicitEmptyAuthoritative(t *testing.T) {
	sources := []WorkspaceSourceInput{}
	req := &CreateTaskRequest{
		Repositories:     []TaskRepositoryInput{{RepositoryID: "inherited-repository", BaseBranch: "main"}},
		WorkspacePath:    "/work/legacy",
		WorkspaceSources: &sources,
	}

	// The transport adapters reject the legacy-field combination before this
	// helper. The service helper still owns the same invariant for trusted
	// callers and must never silently choose one source representation.
	require.ErrorIs(t, normalizeWorkspaceSourcesForCreation(req), ErrInvalidWorkspaceSource)

	req.Repositories = nil
	req.WorkspacePath = ""
	require.NoError(t, normalizeWorkspaceSourcesForCreation(req))
	require.Empty(t, req.Repositories)
}

func TestNormalizeWorkspaceSourcesForCreationRejectsInheritedEdits(t *testing.T) {
	sources := []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir()}}
	req := &CreateTaskRequest{
		ParentID:         "parent-task",
		WorkspaceSources: &sources,
		WorkspacePolicy:  &WorkspacePolicy{Mode: workspaceModeInheritParent},
	}

	require.ErrorIs(t, normalizeWorkspaceSourcesForCreation(req), ErrInvalidWorkspaceSource)
}

func TestNormalizeWorkspaceSourcesForCreationPreservesCheckoutOptions(t *testing.T) {
	options := &models.RepositoryCheckoutOptions{
		Version:           1,
		DownloadMode:      models.DownloadOnDemand,
		SparseDirectories: []string{"extensions/shared"},
	}
	sources := []WorkspaceSourceInput{{
		Kind:            WorkspaceSourceRepository,
		GitHubURL:       "https://github.com/acme/repository",
		BaseBranch:      "main",
		CheckoutOptions: options,
	}}
	req := &CreateTaskRequest{WorkspaceSources: &sources}

	require.NoError(t, normalizeWorkspaceSourcesForCreation(req))
	require.Len(t, req.Repositories, 1)
	require.Equal(t, options, req.Repositories[0].CheckoutOptions)
}
