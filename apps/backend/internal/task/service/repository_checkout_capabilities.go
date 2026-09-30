package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type RepositoryCheckoutCapabilities struct {
	OnDemand bool   `json:"on_demand"`
	Sparse   bool   `json:"sparse"`
	Reason   string `json:"reason,omitempty"`
}

type repositoryCheckoutCapabilityProber interface {
	SupportsRepositoryCheckoutOptions(executorType, prepareScript string) (onDemand, sparse bool)
}

func (s *Service) GetRepositoryCheckoutCapabilities(ctx context.Context, workspaceID string, input TaskRepositoryInput, profileID string) (RepositoryCheckoutCapabilities, error) {
	unsupported := RepositoryCheckoutCapabilities{}
	if _, err := s.GetWorkspace(ctx, workspaceID); err != nil {
		return unsupported, err
	}
	provider, err := s.checkoutOptionsProvider(ctx, workspaceID, input)
	if err != nil {
		return unsupported, err
	}
	if provider != githubProviderName {
		unsupported.Reason = "provider_unsupported"
		return unsupported, nil
	}
	if profileID == "" {
		unsupported.Reason = "executor_required"
		return unsupported, nil
	}
	profile, err := s.executors.GetExecutorProfile(ctx, profileID)
	if err != nil {
		return unsupported, err
	}
	executor, err := s.executors.GetExecutor(ctx, profile.ExecutorID)
	if err != nil {
		return unsupported, err
	}
	if s.checkoutCredentialPolicy != nil {
		managed, err := s.checkoutCredentialPolicy(ctx, workspaceID)
		if err != nil {
			return unsupported, err
		}
		if !managed {
			unsupported.Reason = "credentials_unsupported"
			return unsupported, nil
		}
	}
	if prober, ok := s.executorCapabilityProber.(repositoryCheckoutCapabilityProber); ok {
		unsupported.OnDemand, unsupported.Sparse = prober.SupportsRepositoryCheckoutOptions(string(executor.Type), profile.PrepareScript)
	}
	if !unsupported.OnDemand || !unsupported.Sparse {
		unsupported.Reason = "preparation_unsupported"
	}
	return unsupported, nil
}

func (s *Service) checkoutOptionsProvider(ctx context.Context, workspaceID string, input TaskRepositoryInput) (string, error) {
	if err := s.validateRepositoryCheckoutInput(ctx, workspaceID, input); err != nil {
		return "", err
	}
	if input.LocalPath != "" {
		return sourceTypeLocal, nil
	}
	if input.RepositoryID != "" {
		repo, err := s.repoEntities.GetRepository(ctx, input.RepositoryID)
		if err != nil {
			return "", err
		}
		if repo == nil || repo.WorkspaceID != workspaceID {
			return "", repoerrors.ErrRepositoryNotFound
		}
		if repo.SourceType == sourceTypeLocal {
			return sourceTypeLocal, nil
		}
		return repo.Provider, nil
	}
	provider, _, _, _, err := parseRemoteRepositoryURL(effectiveRemoteURL(input), input.Provider)
	return provider, err
}

func (s *Service) validateTaskCheckoutCapabilities(ctx context.Context, req *CreateTaskRequest) error {
	profileID, _ := req.Metadata[models.MetaKeyExecutorProfileID].(string)
	for _, input := range req.Repositories {
		options, err := models.NormalizeRepositoryCheckoutOptions(input.CheckoutOptions)
		if err != nil {
			return err
		}
		if options == nil || (options.DownloadMode == models.DownloadStandard && len(options.SparseDirectories) == 0) {
			continue
		}
		capabilities, err := s.GetRepositoryCheckoutCapabilities(ctx, req.WorkspaceID, input, profileID)
		if err != nil {
			return err
		}
		if options.DownloadMode == models.DownloadOnDemand && !capabilities.OnDemand || len(options.SparseDirectories) > 0 && !capabilities.Sparse {
			return fmt.Errorf("repository checkout options unavailable: %s", capabilities.Reason)
		}
	}
	return nil
}

// SetRepositoryCheckoutCredentialPolicy provides integration-owned credential routing.
func (s *Service) SetRepositoryCheckoutCredentialPolicy(policy func(context.Context, string) (bool, error)) {
	s.checkoutCredentialPolicy = policy
}
