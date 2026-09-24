package handlers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

func TestParseHTTPTaskWorkspaceSourcesPreservesFreshBranchFields(t *testing.T) {
	sources, err := parseHTTPTaskWorkspaceSources([]json.RawMessage{json.RawMessage(`{
		"kind":"repository",
		"repository_id":"repo-1",
		"base_branch":"develop",
		"checkout_options":{"version":1,"download_mode":"on_demand","sparse_directories":["extensions/shared"]},
		"fresh_branch":true,
		"new_branch_name":"feature/task",
		"confirm_discard":true,
		"consented_dirty_files":["src/app.ts"]
	}`)})

	require.NoError(t, err)
	require.Len(t, sources, 1)
	require.True(t, sources[0].FreshBranch)
	require.Equal(t, "feature/task", sources[0].NewBranchName)
	require.True(t, sources[0].ConfirmDiscard)
	require.Equal(t, []string{"src/app.ts"}, sources[0].ConsentedDirtyFiles)
	require.NotNil(t, sources[0].CheckoutOptions)
	require.Equal(t, 1, sources[0].CheckoutOptions.Version)
	require.Equal(t, "on_demand", sources[0].CheckoutOptions.DownloadMode)
	require.Equal(t, []string{"extensions/shared"}, sources[0].CheckoutOptions.SparseDirectories)
}

func TestParseHTTPWorkspaceSourcesRejectsFreshBranchFields(t *testing.T) {
	_, err := parseHTTPWorkspaceSources([]json.RawMessage{json.RawMessage(`{
		"kind":"repository",
		"repository_id":"repo-1",
		"fresh_branch":true
	}`)})

	require.EqualError(t, err, `field "fresh_branch" is not allowed for repository source`)
}

func TestWorkspaceSourceRepositoryInputsSkipsFoldersAndMapsFreshBranchFields(t *testing.T) {
	inputs, repos := workspaceSourceRepositoryInputs([]service.WorkspaceSourceInput{
		{Kind: service.WorkspaceSourceFolder, LocalPath: "/work/assets"},
		{
			Kind:                service.WorkspaceSourceRepository,
			RepositoryID:        "repo-1",
			BaseBranch:          "develop",
			BranchPolicyID:      "policy-1",
			CheckoutOptions:     &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: "on_demand"},
			FreshBranch:         true,
			NewBranchName:       "feature/task",
			ConfirmDiscard:      true,
			ConsentedDirtyFiles: []string{"src/app.ts"},
		},
	})

	require.Len(t, inputs, 1)
	require.Len(t, repos, 1)
	require.Equal(t, "repo-1", inputs[0].RepositoryID)
	require.True(t, inputs[0].FreshBranch)
	require.Equal(t, "feature/task", inputs[0].NewBranchName)
	require.Equal(t, "repo-1", repos[0].RepositoryID)
	require.Equal(t, "develop", repos[0].BaseBranch)
	require.Equal(t, "policy-1", repos[0].BranchPolicyID)
	require.NotNil(t, repos[0].CheckoutOptions)
	require.Equal(t, "on_demand", repos[0].CheckoutOptions.DownloadMode)
}
