package workflowsync

import (
	"context"

	"go.uber.org/zap"
)

func (s *Service) upsertHostBoundConfig(ctx context.Context, workspaceID string, req *SetConfigRequest) (*Config, error) {
	if req.Provider != ProviderGitHub || s.githubClients == nil {
		return s.store.UpsertConfigForWorkspace(ctx, workspaceID, req)
	}
	bound, err := s.githubClients.BindWorkspaceGitHubHost(ctx, workspaceID, req.GitHubHost)
	if err != nil {
		return nil, err
	}
	var config *Config
	err = s.githubClients.CommitWorkspaceGitHubHost(bound, workspaceID, func(commitCtx context.Context) error {
		var saveErr error
		config, saveErr = s.store.UpsertConfigForWorkspace(commitCtx, workspaceID, req)
		return saveErr
	})
	return config, err
}

func (s *Service) applyFetchedFiles(ctx context.Context, cfg *Config, files []fetchedFile, mode syncMode) (*SyncResult, error) {
	workspaceID := cfg.WorkspaceID
	wasRecovering := cfg.ConsecutiveFailures > 0 || cfg.PollSuspended
	previousFailureClass := cfg.LastErrorClass
	parsed, warnings := parseFiles(files)
	applied, err := s.applier.ApplySyncedWorkflows(ctx, workspaceID, parsed)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, applied.Warnings...)
	successState := cfg.circuitState()
	successState.RecordSuccess()
	persistCtx, cancel := terminalSyncPersistenceContext(ctx, mode)
	defer cancel()
	if err := s.store.RecordSyncStatus(
		persistCtx, workspaceID, true, "", warnings, contentHash(files), s.now().UTC(), successState,
	); err != nil {
		return nil, err
	}
	if wasRecovering {
		incWorkflowSyncTransition("recovered", cfg.Provider, previousFailureClass, "")
		s.logger.Info("workflow sync polling recovered",
			zap.String("workspace_id", workspaceID),
			zap.String("provider", cfg.Provider),
			zap.String("previous_failure_class", previousFailureClass))
	}
	return &SyncResult{
		Created:   applied.Created,
		Updated:   applied.Updated,
		Deleted:   applied.Deleted,
		Warnings:  warnings,
		Unchanged: len(applied.Created)+len(applied.Updated)+len(applied.Deleted) == 0 && len(warnings) == 0,
	}, nil
}
