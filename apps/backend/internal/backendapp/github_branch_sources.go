package backendapp

import (
	"context"
	"fmt"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

func (a codeHostBranchListerAdapter) listGitHubRepoBranches(ctx context.Context, source taskservice.RemoteBranchSource) ([]taskservice.Branch, error) {
	if a.github == nil {
		return nil, fmt.Errorf("GitHub branch provider is unavailable")
	}
	ctx, err := a.github.BindWorkspaceGitHubHost(ctx, source.WorkspaceID, source.ProviderHost)
	if err != nil {
		return nil, err
	}
	remote, err := a.github.ListRepoBranchesForWorkspace(ctx, source.WorkspaceID, source.Owner, source.Name)
	if err != nil {
		return nil, err
	}
	var branches []taskservice.Branch
	err = a.github.CommitWorkspaceGitHubHost(ctx, source.WorkspaceID, func(context.Context) error {
		branches = make([]taskservice.Branch, 0, len(remote))
		for _, branch := range remote {
			branches = append(branches, taskservice.Branch{Name: branch.Name, Type: codeHostRemoteBranchType})
		}
		return nil
	})
	return branches, err
}
