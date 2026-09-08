package service

import (
	"context"
	"errors"
	"strings"
)

// RepositorySelectionResolver verifies a first-use, plugin-owned repository
// URL and returns the complete trusted descriptor to persist. It must not
// perform repository or task writes.
type RepositorySelectionResolver interface {
	ResolveRepositorySelection(context.Context, string, TaskRepositoryInput) (TaskRepositoryInput, error)
}

// RepositorySelectionErrorCode is the stable transport classification for a
// failed server-side repository selection.
type RepositorySelectionErrorCode string

const (
	RepositorySelectionErrorInvalid     RepositorySelectionErrorCode = "repository_selection_invalid"
	RepositorySelectionErrorNotFound    RepositorySelectionErrorCode = "repository_selection_not_found"
	RepositorySelectionErrorUnavailable RepositorySelectionErrorCode = "repository_selection_unavailable"
)

// RepositorySelectionError is safe to expose to task transports. Its Error
// method is bounded and never includes plugin response data.
type RepositorySelectionError struct {
	Code  RepositorySelectionErrorCode
	cause error
}

func (e *RepositorySelectionError) Error() string {
	switch e.Code {
	case RepositorySelectionErrorInvalid:
		return "The selected repository could not be verified."
	case RepositorySelectionErrorNotFound:
		return "The selected repository was not found."
	case RepositorySelectionErrorUnavailable:
		return "The repository provider is unavailable. Check the connection and try again."
	default:
		return "The selected repository could not be resolved."
	}
}

func (e *RepositorySelectionError) Unwrap() error { return e.cause }

// NewRepositorySelectionError creates a safe typed error for adapters that
// translate provider-specific failures into the task-service contract.
func NewRepositorySelectionError(code RepositorySelectionErrorCode, cause error) error {
	return &RepositorySelectionError{Code: code, cause: cause}
}

func (s *Service) preflightRepositorySelections(ctx context.Context, req *CreateTaskRequest) error {
	return s.preflightRepositoryInputs(ctx, req.WorkspaceID, req.Repositories)
}

func (s *Service) preflightRepositoryInputs(
	ctx context.Context, workspaceID string, inputs []TaskRepositoryInput,
) error {
	for index, input := range inputs {
		needsInspection, err := repositorySelectionNeedsInspection(input)
		if err != nil {
			return err
		}
		if !needsInspection {
			continue
		}
		if s.repositorySelectionResolver == nil {
			return NewRepositorySelectionError(RepositorySelectionErrorUnavailable, nil)
		}
		resolved, err := s.repositorySelectionResolver.ResolveRepositorySelection(ctx, workspaceID, input)
		if err != nil {
			return normalizeRepositorySelectionError(err)
		}
		resolved, err = trustResolvedRepositorySelection(input, resolved)
		if err != nil {
			return err
		}
		inputs[index] = resolved
	}
	return nil
}

func repositorySelectionNeedsInspection(input TaskRepositoryInput) (bool, error) {
	if input.RepositoryID != "" || input.TrustedProviderDescriptor || effectiveRemoteURL(input) == "" {
		return false, nil
	}

	provider := strings.TrimSpace(input.Provider)
	if provider != "" &&
		!strings.EqualFold(provider, providerGitHub) &&
		!strings.EqualFold(provider, providerGitLab) &&
		!strings.EqualFold(provider, providerAzureDevOps) {
		return true, nil
	}

	parsedProvider, _, _, _, err := parseRemoteRepositoryURL(effectiveRemoteURL(input), provider)
	if err != nil {
		return false, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	if provider != "" && !strings.EqualFold(parsedProvider, provider) {
		return false, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	return false, nil
}

func trustResolvedRepositorySelection(
	requested, resolved TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	if resolved.RepositoryID != "" || !resolved.TrustedProviderDescriptor {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	if !strings.EqualFold(strings.TrimSpace(requested.Provider), strings.TrimSpace(resolved.Provider)) {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	if hint := strings.TrimSpace(requested.ProviderScope); hint != "" && hint != strings.TrimSpace(resolved.ProviderScope) {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	if hint := strings.TrimSpace(requested.ProviderRepoID); hint != "" && hint != strings.TrimSpace(resolved.ProviderRepoID) {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	if err := validateTrustedRemoteRepository(resolved); err != nil || strings.TrimSpace(resolved.DefaultBranch) == "" {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, err)
	}
	// Branches and policy selectors belong to the task request. The resolver
	// may only replace provider identity and the exact clone URL.
	resolved.RepositoryID = ""
	resolved.BaseBranch = requested.BaseBranch
	resolved.CheckoutBranch = requested.CheckoutBranch
	resolved.BranchPolicyID = requested.BranchPolicyID
	resolved.PRNumber = requested.PRNumber
	resolved.RemoteContribution = requested.RemoteContribution
	resolved.ContributionDestination = requested.ContributionDestination
	resolved.PreserveBaseBranch = requested.PreserveBaseBranch
	resolved.ResolveProviderDefaults = requested.ResolveProviderDefaults
	resolved.TrustedProviderDescriptor = true
	resolved.GitHubURL = ""
	resolved.LocalPath = ""
	return resolved, nil
}

func normalizeRepositorySelectionError(err error) error {
	var selectionErr *RepositorySelectionError
	if errors.As(err, &selectionErr) {
		return err
	}
	return NewRepositorySelectionError(RepositorySelectionErrorUnavailable, err)
}
