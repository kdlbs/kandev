package service

import (
	"testing"

	"github.com/stretchr/testify/require"
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
