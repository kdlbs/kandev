package github

import (
	"context"
	"encoding/json"
	"fmt"
)

type ciRunActionsMetadataClient interface {
	RerunFailedActionsJobsWithMetadata(
		context.Context, string, string, int64,
	) (GitHubRequestMetadata, error)
	DispatchActionsWorkflowWithMetadata(
		context.Context, string, string, int64, string, map[string]string,
	) (GitHubRequestMetadata, error)
}

type tokenPrincipalProvider interface {
	Principal() TokenPrincipal
}

func ciRunPrincipalForClient(client ciRunActionsClient, workspaceID string) AuthPrincipal {
	principal := AuthPrincipal{
		Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
		WorkspaceID: workspaceID,
	}
	provider, ok := client.(tokenPrincipalProvider)
	if !ok {
		return principal
	}
	tokenPrincipal := provider.Principal()
	principal.Login = tokenPrincipal.Login
	principal.InstallationID = tokenPrincipal.InstallationID
	principal.AppRegistrationID = tokenPrincipal.AppRegistrationID
	principal.AppCredentialGeneration = tokenPrincipal.AppCredentialGeneration
	return principal
}

func setCIRunProviderPrincipal(request *CIRunRequest, principal AuthPrincipal) {
	encoded, err := json.Marshal(principal)
	if err == nil {
		request.ProviderPrincipalJSON = string(encoded)
	}
}

func decodeCIRunProviderPrincipal(raw string) *AuthPrincipal {
	if raw == "" {
		return nil
	}
	var principal AuthPrincipal
	if err := json.Unmarshal([]byte(raw), &principal); err != nil || principal.Kind == "" {
		return nil
	}
	return &principal
}

func ciRunProviderPrincipalAudit(raw string) map[string]any {
	principal := decodeCIRunProviderPrincipal(raw)
	if principal == nil {
		return nil
	}
	return map[string]any{
		"kind": principal.Kind, "source": principal.Source, "login": principal.Login,
		"installation_id":     principal.InstallationID,
		"app_registration_id": principal.AppRegistrationID,
		"app_generation":      principal.AppCredentialGeneration,
		"workspace_id":        principal.WorkspaceID,
	}
}

func rerunFailedCIRunJobs(
	ctx context.Context, client ciRunActionsClient, owner, repo string, runID int64,
) (GitHubRequestMetadata, error) {
	if metadataClient, ok := client.(ciRunActionsMetadataClient); ok {
		return metadataClient.RerunFailedActionsJobsWithMetadata(ctx, owner, repo, runID)
	}
	return GitHubRequestMetadata{}, client.RerunFailedActionsJobs(ctx, owner, repo, runID)
}

func dispatchCIRunWorkflow(
	ctx context.Context,
	client ciRunActionsClient,
	owner, repo string,
	workflowID int64,
	ref string,
	inputs map[string]string,
) (GitHubRequestMetadata, error) {
	if metadataClient, ok := client.(ciRunActionsMetadataClient); ok {
		return metadataClient.DispatchActionsWorkflowWithMetadata(
			ctx, owner, repo, workflowID, ref, inputs,
		)
	}
	return GitHubRequestMetadata{}, client.DispatchActionsWorkflow(ctx, owner, repo, workflowID, ref, inputs)
}

func applyCIRunProviderMetadata(request *CIRunRequest, metadata GitHubRequestMetadata, err error) {
	if metadata.RequestID != "" {
		request.ProviderRequestID = metadata.RequestID
	}
	if metadata.URL != "" {
		request.ProviderURL = metadata.URL
	}
	if providerErr, ok := err.(*CIRunProviderError); ok {
		if request.ProviderRequestID == "" {
			request.ProviderRequestID = providerErr.RequestID
		}
		if request.ProviderURL == "" {
			request.ProviderURL = providerErr.URL
		}
	}
}

func ciRunRerunProviderURL(owner, repo string, runID int64) string {
	return fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/rerun-failed-jobs", githubAPIBase, owner, repo, runID)
}

func ciRunDispatchProviderURL(owner, repo string, workflowID int64) string {
	return fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%d/dispatches", githubAPIBase, owner, repo, workflowID)
}
