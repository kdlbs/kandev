package sentry

import (
	"context"
	"errors"
	"fmt"
)

// ErrSameWorkspace is returned when a copy targets the same workspace it reads
// from.
var ErrSameWorkspace = errors.New("sentry: source and target workspaces are the same")

// ErrNothingToCopy is returned when the source workspace has no Sentry
// instances.
var ErrNothingToCopy = errors.New("sentry: source workspace has no Sentry instances to copy")

// CopyConfigToWorkspace copies every Sentry instance from sourceWorkspaceID
// into targetWorkspaceID: fresh instance IDs, secrets duplicated under the new
// per-instance keys, and names deduped against the target's existing instances.
// Issue watches are intentionally out of scope — only the connection settings
// and secrets are duplicated. Returns the newly created instances.
func (s *Service) CopyConfigToWorkspace(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) ([]*SentryConfig, error) {
	if sourceWorkspaceID == "" || targetWorkspaceID == "" {
		return nil, fmt.Errorf("%w: source and target workspace IDs are required", ErrInvalidConfig)
	}
	if sourceWorkspaceID == targetWorkspaceID {
		return nil, ErrSameWorkspace
	}
	sources, err := s.store.ListInstances(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source sentry instances: %w", err)
	}
	if len(sources) == 0 {
		return nil, ErrNothingToCopy
	}
	existing, err := s.store.ListInstances(ctx, targetWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read target sentry instances: %w", err)
	}
	used := make(map[string]struct{}, len(existing))
	for _, inst := range existing {
		used[inst.Name] = struct{}{}
	}
	copied := make([]*SentryConfig, 0, len(sources))
	for _, src := range sources {
		out, err := s.copyInstance(ctx, targetWorkspaceID, src, used)
		if err != nil {
			return nil, err
		}
		copied = append(copied, out)
	}
	return copied, nil
}

// copyInstance duplicates one source instance into the target workspace with a
// name deduped against used (which it updates), its secret rekeyed under a fresh
// per-instance key, and an async health probe kicked off.
func (s *Service) copyInstance(ctx context.Context, targetWorkspaceID string, src *SentryConfig, used map[string]struct{}) (*SentryConfig, error) {
	cfg := &SentryConfig{
		WorkspaceID: targetWorkspaceID,
		Name:        uniqueInstanceName(used, src.Name),
		AuthMethod:  src.AuthMethod,
		URL:         src.URL,
	}
	if err := s.store.CreateInstance(ctx, cfg); err != nil {
		return nil, fmt.Errorf("create copied sentry instance: %w", err)
	}
	if s.secrets != nil {
		secret, ok, err := s.revealInstanceSecret(ctx, src.ID)
		if err != nil {
			return nil, fmt.Errorf("read source sentry secret: %w", err)
		}
		if ok {
			if err := s.secrets.Set(ctx, secretKeyForInstance(cfg.ID), "Sentry auth token", secret); err != nil {
				return nil, fmt.Errorf("store copied sentry secret: %w", err)
			}
		}
	}
	s.invalidateClient(cfg.ID)
	go s.RecordAuthHealthForInstance(context.Background(), cfg.ID)
	return s.GetInstance(ctx, targetWorkspaceID, cfg.ID)
}
