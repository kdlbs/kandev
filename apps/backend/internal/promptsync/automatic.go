package promptsync

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
)

// SyncIfDue runs an automatic sync when the configuration is due. It reads
// the source without holding s.mu, so a read deferred by GitHub admission can
// wait without blocking manual syncs, saves, or removal. The result applies
// only if the configuration is unchanged when the read ends. Failures are
// recorded on the configuration and logged, never returned.
func (s *Service) SyncIfDue(ctx context.Context) {
	snapshot := s.dueSnapshot(ctx)
	if snapshot == nil {
		return
	}
	run := s.fetchRun(ctx, syncAutomatic, snapshot, true)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	cfg := s.currentAutomaticTarget(ctx, snapshot)
	if cfg == nil {
		s.logger.Info("prompt sync automatic result superseded", zap.String("provider", snapshot.Provider))
		return
	}
	if run.err != nil {
		s.recordRunError(ctx, cfg, syncAutomatic, run.err)
		return
	}
	_, _ = s.applyRun(run, syncAutomatic, cfg, nil)
}

// dueSnapshot returns the configuration when an automatic sync is due, or nil.
func (s *Service) dueSnapshot(ctx context.Context) *Config {
	if !s.mu.TryLock() {
		return nil
	}
	defer s.mu.Unlock()
	cfg, err := s.store.Get(ctx)
	if err != nil {
		s.logger.Warn("failed to load prompt sync config", zap.Error(err))
		return nil
	}
	if cfg == nil {
		return nil
	}
	s.refreshCredentialFingerprint(ctx, cfg)
	if cfg.PollSuspended {
		incPollSkipped("suspended")
		return nil
	}
	if !cfg.due(s.now().UTC()) {
		return nil
	}
	return cfg
}

// currentAutomaticTarget reloads the configuration and returns it when it is
// still the one snapshot was taken from, or nil. The caller holds s.mu. Saves
// and recorded results change UpdatedAt, removal deletes the row, and a
// recovery reset follows a fingerprint change.
func (s *Service) currentAutomaticTarget(ctx context.Context, snapshot *Config) *Config {
	cfg, err := s.store.Get(ctx)
	if err != nil {
		s.logger.Warn("failed to load prompt sync config", zap.Error(err))
		return nil
	}
	if cfg == nil {
		return nil
	}
	s.refreshCredentialFingerprint(ctx, cfg)
	if !cfg.UpdatedAt.Equal(snapshot.UpdatedAt) || cfg.CredentialFingerprint != snapshot.CredentialFingerprint ||
		cfg.source() != snapshot.source() || cfg.GitHubHost != snapshot.GitHubHost {
		return nil
	}
	return snapshot
}

// retryDeferred runs read. When wait is set and GitHub admission defers the
// read, it waits for an admission change or the retry boundary and runs the
// read again.
func retryDeferred[T any](ctx context.Context, wait bool, read func() (T, error)) (T, error) {
	for {
		value, err := read()
		var deferred *github.AdmissionDeferredError
		if !wait || !errors.As(err, &deferred) {
			return value, err
		}
		if err := deferred.Wait(ctx); err != nil {
			var zero T
			return zero, err
		}
	}
}
