package sentry

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// ErrSameWorkspace is returned when a copy targets the same workspace it reads
// from.
var ErrSameWorkspace = errors.New("sentry: source and target workspaces are the same")

// ErrNothingToCopy is returned when the source workspace has no Sentry
// instances.
var ErrNothingToCopy = errors.New("sentry: source workspace has no Sentry instances to copy")

type copyInput struct {
	config    *SentryConfig
	secret    string
	hasSecret bool
}

// CopyConfigToWorkspace copies every Sentry instance from sourceWorkspaceID
// into targetWorkspaceID: fresh instance IDs, secrets duplicated under the new
// per-instance keys, and names deduped against the target's existing instances.
// Issue watches are intentionally out of scope — only the connection settings
// and secrets are duplicated. Returns the newly created instances.
func (s *Service) CopyConfigToWorkspace(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) ([]*SentryConfig, error) {
	if err := validateCopyWorkspaces(sourceWorkspaceID, targetWorkspaceID); err != nil {
		return nil, err
	}
	prepared, used, err := s.prepareCopyInputs(ctx, sourceWorkspaceID, targetWorkspaceID)
	if err != nil {
		return nil, err
	}
	copied, err := s.createCopiedInstances(ctx, targetWorkspaceID, prepared, used)
	if err != nil {
		return nil, err
	}
	s.finalizeCopiedInstances(copied)
	return copied, nil
}

// validateCopyWorkspaces rejects empty or identical source/target workspace
// IDs before any store access.
func validateCopyWorkspaces(sourceWorkspaceID, targetWorkspaceID string) error {
	if sourceWorkspaceID == "" || targetWorkspaceID == "" {
		return fmt.Errorf("%w: source and target workspace IDs are required", ErrInvalidConfig)
	}
	if sourceWorkspaceID == targetWorkspaceID {
		return ErrSameWorkspace
	}
	return nil
}

// prepareCopyInputs reads every source instance and, for each, reveals its
// secret before any target-workspace mutation, plus the target's existing
// instance names to dedupe against. Aborting here means no partial target
// state to roll back.
func (s *Service) prepareCopyInputs(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) ([]copyInput, map[string]struct{}, error) {
	sources, err := s.store.ListInstances(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("read source sentry instances: %w", err)
	}
	if len(sources) == 0 {
		return nil, nil, ErrNothingToCopy
	}
	existing, err := s.store.ListInstances(ctx, targetWorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("read target sentry instances: %w", err)
	}
	used := make(map[string]struct{}, len(existing))
	for _, inst := range existing {
		used[inst.Name] = struct{}{}
	}
	prepared := make([]copyInput, 0, len(sources))
	for _, src := range sources {
		input := copyInput{config: src}
		if s.secrets != nil {
			secret, ok, err := s.revealInstanceSecret(ctx, src.ID)
			if err != nil {
				return nil, nil, fmt.Errorf("read source sentry secret: %w", err)
			}
			input.secret = secret
			input.hasSecret = ok
		}
		prepared = append(prepared, input)
	}
	return prepared, used, nil
}

// createCopiedInstances copies every prepared input into the target
// workspace, rolling back all instances created by this request if any copy
// fails.
func (s *Service) createCopiedInstances(ctx context.Context, targetWorkspaceID string, prepared []copyInput, used map[string]struct{}) ([]*SentryConfig, error) {
	copied := make([]*SentryConfig, 0, len(prepared))
	for _, input := range prepared {
		out, err := s.copyInstance(ctx, targetWorkspaceID, input, used)
		if out != nil {
			copied = append(copied, out)
		}
		if err != nil {
			return nil, s.rollbackCopiedInstances(ctx, copied, err)
		}
	}
	return copied, nil
}

// finalizeCopiedInstances invalidates cached clients and kicks off async
// health probes only after every instance in the request has been copied.
func (s *Service) finalizeCopiedInstances(copied []*SentryConfig) {
	for _, cfg := range copied {
		s.invalidateClient(cfg.ID)
	}
	for _, cfg := range copied {
		go s.RecordAuthHealthForInstance(context.Background(), cfg.ID)
	}
}

// copyInstance duplicates a prepared source instance into the target workspace
// with a name deduped against used (which it updates) and a rekeyed secret. The
// caller starts health probes after every copy succeeds.
func (s *Service) copyInstance(ctx context.Context, targetWorkspaceID string, input copyInput, used map[string]struct{}) (*SentryConfig, error) {
	cfg := &SentryConfig{
		WorkspaceID: targetWorkspaceID,
		Name:        uniqueInstanceName(used, input.config.Name),
		AuthMethod:  input.config.AuthMethod,
		URL:         input.config.URL,
	}
	if err := s.store.CreateInstance(ctx, cfg); err != nil {
		return nil, fmt.Errorf("create copied sentry instance: %w", err)
	}
	if input.hasSecret && s.secrets != nil {
		if err := s.secrets.Set(ctx, secretKeyForInstance(cfg.ID), "Sentry auth token", input.secret); err != nil {
			return cfg, fmt.Errorf("store copied sentry secret: %w", err)
		}
		cfg.HasSecret = true
	}
	return cfg, nil
}

// rollbackCopiedInstances removes only instances created by the current copy
// request. It continues after cleanup errors so every created row is attempted.
func (s *Service) rollbackCopiedInstances(ctx context.Context, copied []*SentryConfig, cause error) error {
	cleanupCtx := context.WithoutCancel(ctx)
	var rollbackErrs []error
	for i := len(copied) - 1; i >= 0; i-- {
		cfg := copied[i]
		if cfg == nil || cfg.ID == "" {
			continue
		}
		if err := s.store.DeleteInstance(cleanupCtx, cfg.ID); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback copied sentry instance %q: %w", cfg.ID, err))
		}
		if s.secrets != nil {
			key := secretKeyForInstance(cfg.ID)
			exists, err := s.secrets.Exists(cleanupCtx, key)
			if err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("check copied sentry secret %q during rollback: %w", cfg.ID, err))
			} else if exists {
				if err := s.secrets.Delete(cleanupCtx, key); err != nil {
					rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback copied sentry secret %q: %w", cfg.ID, err))
				}
			}
		}
	}
	if len(rollbackErrs) == 0 {
		return cause
	}
	cleanupErr := errors.Join(rollbackErrs...)
	s.log.Warn("sentry: copy rollback cleanup failed", zap.Error(cleanupErr))
	return errors.Join(cause, cleanupErr)
}
