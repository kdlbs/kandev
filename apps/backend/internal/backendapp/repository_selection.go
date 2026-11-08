package backendapp

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/plugins"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type repositoryProviderInspector interface {
	InspectRepositoryProvider(context.Context, string, plugins.RepositoryProviderInspectionRequest) (*plugins.RepositoryProviderInspection, error)
	DiscoverRepositoryProviders(context.Context, string, plugins.RepositoryProviderDiscoveryRequest) (*plugins.RepositoryProviderDiscoveryResult, error)
}

type pluginRepositorySelectionResolver struct {
	inspector repositoryProviderInspector
}

func (r pluginRepositorySelectionResolver) ResolveRepositorySelection(
	ctx context.Context, workspaceID string, input taskservice.TaskRepositoryInput,
) (taskservice.TaskRepositoryInput, error) {
	if r.inspector == nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(
			taskservice.RepositorySelectionErrorUnavailable, nil,
		)
	}
	url := strings.TrimSpace(input.RemoteURL)
	if url == "" {
		url = strings.TrimSpace(input.GitHubURL)
	}
	inspection, err := r.inspector.InspectRepositoryProvider(ctx, workspaceID, plugins.RepositoryProviderInspectionRequest{
		Provider: input.Provider, URL: url, ProviderScope: input.ProviderScope,
		ProviderRepositoryID: input.ProviderRepoID,
	})
	if err != nil {
		return taskservice.TaskRepositoryInput{}, mapRepositoryProviderError(err)
	}
	if inspection == nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(
			taskservice.RepositorySelectionErrorUnavailable, nil,
		)
	}
	return taskservice.TaskRepositoryInput{
		BaseBranch: input.BaseBranch, CheckoutBranch: input.CheckoutBranch,
		BranchPolicyID: input.BranchPolicyID, PRNumber: input.PRNumber,
		RemoteContribution: input.RemoteContribution, ContributionDestination: input.ContributionDestination,
		PreserveBaseBranch: input.PreserveBaseBranch, ResolveProviderDefaults: input.ResolveProviderDefaults,
		RemoteURL: inspection.CloneURL, Provider: inspection.ProviderID,
		ProviderHost: inspection.ProviderHost, ProviderScope: inspection.ProviderScope,
		ProviderRepoID: inspection.ProviderRepositoryID, ProviderOwner: inspection.OwnerOrProject,
		ProviderName: inspection.Name, DefaultBranch: inspection.DefaultBranch,
		TrustedProviderDescriptor: true,
	}, nil
}

// DiscoverRepositorySelections maps every validated plugin descriptor without
// rebuilding its clone URL. Branch and policy selectors stay caller-owned.
func (r pluginRepositorySelectionResolver) DiscoverRepositorySelections(
	ctx context.Context, workspaceID string, input taskservice.TaskRepositoryInput,
) (taskservice.RepositorySelectionDiscoveryResult, error) {
	if r.inspector == nil {
		return taskservice.RepositorySelectionDiscoveryResult{}, taskservice.NewRepositorySelectionError(
			taskservice.RepositorySelectionErrorUnavailable, nil,
		)
	}
	url := strings.TrimSpace(input.RemoteURL)
	if url == "" {
		url = strings.TrimSpace(input.GitHubURL)
	}
	discovery, err := r.inspector.DiscoverRepositoryProviders(ctx, workspaceID, plugins.RepositoryProviderDiscoveryRequest{
		URL: url, ProviderScope: input.ProviderScope, ProviderRepositoryID: input.ProviderRepoID,
	})
	if err != nil {
		return taskservice.RepositorySelectionDiscoveryResult{}, mapRepositoryProviderError(err)
	}
	if discovery == nil {
		return taskservice.RepositorySelectionDiscoveryResult{}, taskservice.NewRepositorySelectionError(
			taskservice.RepositorySelectionErrorUnavailable, nil,
		)
	}
	result := taskservice.RepositorySelectionDiscoveryResult{
		Candidates: make([]taskservice.TaskRepositoryInput, 0, len(discovery.Candidates)),
	}
	for _, inspection := range discovery.Candidates {
		result.Candidates = append(result.Candidates, taskservice.TaskRepositoryInput{
			BaseBranch: input.BaseBranch, CheckoutBranch: input.CheckoutBranch,
			BranchPolicyID: input.BranchPolicyID, PRNumber: input.PRNumber,
			RemoteContribution: input.RemoteContribution, ContributionDestination: input.ContributionDestination,
			PreserveBaseBranch: input.PreserveBaseBranch, ResolveProviderDefaults: input.ResolveProviderDefaults,
			RemoteURL: inspection.CloneURL, Provider: inspection.ProviderID,
			ProviderHost: inspection.ProviderHost, ProviderScope: inspection.ProviderScope,
			ProviderRepoID: inspection.ProviderRepositoryID, ProviderOwner: inspection.OwnerOrProject,
			ProviderName: inspection.Name, DefaultBranch: inspection.DefaultBranch,
			TrustedProviderDescriptor: true,
		})
	}
	if discovery.Failure != nil {
		result.Failure = taskservice.NewRepositorySelectionFailure(
			repositorySelectionErrorCode(discovery.Failure), discovery.Failure,
		)
	}
	return result, nil
}

func mapRepositoryProviderError(err error) error {
	var providerErr *plugins.RepositoryProviderError
	if !errors.As(err, &providerErr) {
		return taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, err)
	}
	return taskservice.NewRepositorySelectionError(repositorySelectionErrorCode(providerErr), err)
}

func repositorySelectionErrorCode(providerErr *plugins.RepositoryProviderError) taskservice.RepositorySelectionErrorCode {
	code := taskservice.RepositorySelectionErrorUnavailable
	if providerErr == nil {
		return code
	}
	switch providerErr.Code {
	case plugins.RepositoryProviderErrorInvalid:
		code = taskservice.RepositorySelectionErrorInvalid
	case plugins.RepositoryProviderErrorNotFound:
		code = taskservice.RepositorySelectionErrorNotFound
	}
	return code
}
