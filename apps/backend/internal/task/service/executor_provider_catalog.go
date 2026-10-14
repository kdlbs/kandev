package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

var ErrExecutorProviderUnavailable = errors.New("executor provider is unavailable")

// syncPluginExecutorEntries stores stable, host-owned executor rows so normal
// executor profiles can use the existing durable profile table. The plugin
// catalog remains authoritative for names, schemas, capabilities and health.
func (s *Service) syncPluginExecutorEntries(ctx context.Context) ([]models.ExecutorProvider, error) {
	s.executorProviderCatalogMu.Lock()
	defer s.executorProviderCatalogMu.Unlock()

	providers, err := s.listExecutorProviders(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := s.executors.ListExecutors(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*models.Executor, len(existing))
	for _, executor := range existing {
		if executor != nil {
			byID[executor.ID] = executor
		}
	}
	known := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		if err := validateExecutorProviderIdentity(provider); err != nil {
			return nil, err
		}
		known[provider.ExecutorID] = struct{}{}
		if err := s.syncPluginExecutorProvider(ctx, provider, byID); err != nil {
			return nil, err
		}
	}
	if err := s.disableMissingPluginExecutorEntries(ctx, existing, known); err != nil {
		return nil, err
	}
	return providers, nil
}

func (s *Service) listExecutorProviders(ctx context.Context) ([]models.ExecutorProvider, error) {
	if s.executorProviderCatalog == nil {
		return nil, nil
	}
	return s.executorProviderCatalog.ListExecutorProviders(ctx)
}

func validateExecutorProviderIdentity(provider models.ExecutorProvider) error {
	if provider.ExecutorID == "" || provider.Identity == "" || provider.PluginID == "" || provider.Key == "" {
		return fmt.Errorf("%w: provider catalog returned an incomplete identity", ErrExecutorProviderUnavailable)
	}
	return nil
}

func (s *Service) syncPluginExecutorProvider(ctx context.Context, provider models.ExecutorProvider, byID map[string]*models.Executor) error {
	stored := byID[provider.ExecutorID]
	if stored == nil {
		stored = &models.Executor{
			ID: provider.ExecutorID, Name: provider.DisplayName,
			Type: models.ExecutorTypePluginRemote, IsSystem: true,
			Status: providerStatus(provider), Resumable: provider.Capabilities.Reattach,
			Config: map[string]string{models.ExecutorProviderIDConfigKey: provider.Identity},
		}
		if err := s.executors.CreateExecutor(ctx, stored); err != nil {
			return fmt.Errorf("create provider executor entry: %w", err)
		}
		byID[provider.ExecutorID] = stored
		return nil
	}
	if err := validateStoredExecutorProvider(stored, provider); err != nil {
		return err
	}
	if !pluginExecutorProviderNeedsUpdate(stored, provider) {
		return nil
	}
	stored.Name = provider.DisplayName
	stored.Status = providerStatus(provider)
	stored.Resumable = provider.Capabilities.Reattach
	if err := s.executors.UpdateExecutor(ctx, stored); err != nil {
		return fmt.Errorf("update provider executor entry: %w", err)
	}
	return nil
}

func validateStoredExecutorProvider(stored *models.Executor, provider models.ExecutorProvider) error {
	identity := ""
	if stored.Config != nil {
		identity = stored.Config[models.ExecutorProviderIDConfigKey]
	}
	if stored.Type != models.ExecutorTypePluginRemote || identity != provider.Identity || !stored.IsSystem {
		return fmt.Errorf("%w: provider executor identity conflicts with an existing executor", ErrExecutorProviderUnavailable)
	}
	return nil
}

func pluginExecutorProviderNeedsUpdate(stored *models.Executor, provider models.ExecutorProvider) bool {
	return stored.Name != provider.DisplayName || stored.Status != providerStatus(provider) || stored.Resumable != provider.Capabilities.Reattach
}

func (s *Service) disableMissingPluginExecutorEntries(ctx context.Context, existing []*models.Executor, known map[string]struct{}) error {
	for _, executor := range existing {
		if executor == nil || executor.Type != models.ExecutorTypePluginRemote {
			continue
		}
		if _, declared := known[executor.ID]; !declared && executor.Status != models.ExecutorStatusDisabled {
			executor.Status = models.ExecutorStatusDisabled
			if err := s.executors.UpdateExecutor(ctx, executor); err != nil {
				return fmt.Errorf("disable unavailable provider executor entry: %w", err)
			}
		}
	}
	return nil
}

func providerStatus(provider models.ExecutorProvider) models.ExecutorStatus {
	if provider.Available {
		return models.ExecutorStatusActive
	}
	return models.ExecutorStatusDisabled
}

func (s *Service) attachExecutorProvider(executor *models.Executor, providers []models.ExecutorProvider) {
	if executor == nil || executor.Type != models.ExecutorTypePluginRemote {
		return
	}
	for index := range providers {
		if providers[index].ExecutorID == executor.ID {
			provider := providers[index]
			executor.Provider = &provider
			return
		}
	}
}

// ValidateExecutorProfileAdmission is the shared fail-closed check for every
// path that starts or switches a task onto a provider-owned executor.
func (s *Service) ValidateExecutorProfileAdmission(ctx context.Context, profileID string) error {
	profile, err := s.executors.GetExecutorProfile(ctx, profileID)
	if err != nil {
		return err
	}
	executor, err := s.GetExecutor(ctx, profile.ExecutorID)
	if err != nil {
		return err
	}
	if executor.Type != models.ExecutorTypePluginRemote {
		return nil
	}
	if executor.Status != models.ExecutorStatusActive || executor.Provider == nil || !executor.Provider.Available {
		return fmt.Errorf("%w: %s", ErrExecutorProviderUnavailable, executor.Name)
	}
	return nil
}

func (s *Service) validateRequestedExecutorAdmission(ctx context.Context, req *CreateTaskRequest) error {
	if req == nil {
		return nil
	}
	launchRequested := req.StartAgent || req.DeferredLaunch != nil || (req.StartWhenUnblocked != nil && *req.StartWhenUnblocked)
	if !launchRequested {
		return nil
	}
	if req.ExecutorProfileID != "" {
		if err := s.ValidateExecutorProfileAdmission(ctx, req.ExecutorProfileID); err != nil {
			return err
		}
		profile, err := s.executors.GetExecutorProfile(ctx, req.ExecutorProfileID)
		if err != nil {
			return err
		}
		executor, err := s.GetExecutor(ctx, profile.ExecutorID)
		if err != nil {
			return err
		}
		if executor.Type == models.ExecutorTypePluginRemote {
			return s.validatePluginExecutorRequestCompatibility(ctx, req)
		}
		return nil
	}
	if req.ExecutorID != "" {
		executor, err := s.GetExecutor(ctx, req.ExecutorID)
		if err != nil {
			return err
		}
		if executor.Type == models.ExecutorTypePluginRemote {
			return fmt.Errorf("%w: a provider profile is required", ErrExecutorProviderUnavailable)
		}
	}
	return nil
}

func (s *Service) validatePluginExecutorRequestCompatibility(ctx context.Context, req *CreateTaskRequest) error {
	mode := taskWorkspaceMode(req.Metadata)
	if req.WorkspacePolicy != nil && req.WorkspacePolicy.Mode != "" {
		mode = req.WorkspacePolicy.Mode
	}
	if mode == workspaceModeSharedGroup || mode == workspaceModeInheritParent {
		return fmt.Errorf("%w: plugin executor requires an isolated workspace", ErrInvalidExecutorConfig)
	}
	if req.WorkspacePath != "" {
		return fmt.Errorf("%w: plugin executor cannot use a host workspace path", ErrInvalidExecutorConfig)
	}
	for _, repository := range req.Repositories {
		if err := s.validatePluginExecutorRepositoryCompatibility(ctx, repository); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validatePluginExecutorRepositoryCompatibility(ctx context.Context, repository TaskRepositoryInput) error {
	if repository.LocalPath != "" || isLocalRepositoryURL(repository.RemoteURL) {
		return fmt.Errorf("%w: plugin executor requires cloneable repositories", ErrInvalidExecutorConfig)
	}
	if repository.RepositoryID == "" {
		return nil
	}
	if s.repoEntities == nil {
		return fmt.Errorf("%w: repository cloneability cannot be verified", ErrInvalidExecutorConfig)
	}
	entity, err := s.repoEntities.GetRepository(ctx, repository.RepositoryID)
	if err != nil {
		return fmt.Errorf("%w: repository cloneability cannot be verified", ErrInvalidExecutorConfig)
	}
	if entity == nil || entity.SourceType == sourceTypeLocal || entity.RemoteURL == "" || isLocalRepositoryURL(entity.RemoteURL) {
		return fmt.Errorf("%w: plugin executor requires cloneable repositories", ErrInvalidExecutorConfig)
	}
	return nil
}

func isLocalRepositoryURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "file:") || filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") {
		return true
	}
	if strings.Contains(trimmed, "@") && !strings.Contains(trimmed, "://") {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return true
	}
	return parsed.Scheme != "" && parsed.Scheme != protocolHTTPS && parsed.Scheme != protocolHTTP && parsed.Scheme != "ssh" && parsed.Scheme != "git"
}
