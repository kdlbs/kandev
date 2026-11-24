package promptsync

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
)

// automaticRun is one automatic sync: the configuration snapshot it reads, the
// live credential identity at that time, and the context that superseding
// mutations cancel under s.mu.
type automaticRun struct {
	ctx      context.Context
	cancel   context.CancelFunc
	cfg      *Config
	identity credentialIdentity
}

// credentialIdentity is a live connection fingerprint. known is false when
// the provider has no fingerprint or the lookup failed.
type credentialIdentity struct {
	fingerprint string
	known       bool
}

// SyncIfDue runs an automatic sync when the configuration is due. It reads
// the source without holding s.mu, so a read deferred by GitHub admission can
// wait without blocking manual syncs, saves, or removal. Those mutations
// cancel the run; a run whose credential changed is discarded. Failures are
// recorded on the configuration and logged, never returned.
func (s *Service) SyncIfDue(ctx context.Context) {
	run := s.startAutomatic(ctx)
	if run == nil {
		return
	}
	defer run.cancel()
	fetched := s.fetchRun(run.ctx, syncAutomatic, run.cfg, true)
	if run.ctx.Err() != nil {
		s.logSuperseded(ctx, run)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finishAutomaticLocked(ctx, run) {
		return
	}
	if fetched.err != nil {
		s.recordRunError(ctx, run.cfg, syncAutomatic, fetched.err)
		return
	}
	_, _ = s.applyRun(fetched, syncAutomatic, run.cfg, nil)
}

// startAutomatic returns a run when an automatic sync is due, or nil.
func (s *Service) startAutomatic(ctx context.Context) *automaticRun {
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
	run := &automaticRun{cfg: cfg, identity: s.liveIdentity(ctx, cfg)}
	run.ctx, run.cancel = context.WithCancel(ctx)
	s.automatic = run
	return run
}

// finishAutomaticLocked releases run and reports whether its result may be
// recorded: the run is still current, was neither cancelled nor superseded,
// the configuration still exists, and the live credential is unchanged. The
// caller holds s.mu.
func (s *Service) finishAutomaticLocked(ctx context.Context, run *automaticRun) bool {
	if s.automatic == run {
		s.automatic = nil
	}
	if ctx.Err() != nil {
		return false
	}
	current := run.ctx.Err() == nil
	if current {
		cfg, err := s.store.Get(ctx)
		current = err == nil && cfg != nil && !run.identity.changed(s.liveIdentity(ctx, run.cfg))
	}
	if !current {
		s.logSuperseded(ctx, run)
	}
	return current
}

// logSuperseded logs a discarded run unless the poller is stopping.
func (s *Service) logSuperseded(ctx context.Context, run *automaticRun) {
	if ctx.Err() == nil {
		s.logger.Info("prompt sync automatic result superseded", zap.String("provider", run.cfg.Provider))
	}
}

// supersedeAutomaticLocked cancels the in-flight automatic run, waking any
// admission wait. The caller holds s.mu.
func (s *Service) supersedeAutomaticLocked() {
	if s.automatic != nil {
		s.automatic.cancel()
		s.automatic = nil
	}
}

func (s *Service) liveIdentity(ctx context.Context, cfg *Config) credentialIdentity {
	provider, ok := s.fingerprintProvider(cfg.Provider)
	if !ok {
		return credentialIdentity{}
	}
	fingerprint, err := provider.WorkspaceConnectionFingerprint(ctx, cfg.WorkspaceID)
	if err != nil {
		return credentialIdentity{}
	}
	return credentialIdentity{fingerprint: fingerprint, known: true}
}

// changed reports a known identity that differs, including a removed
// connection whose fingerprint is now empty.
func (c credentialIdentity) changed(now credentialIdentity) bool {
	return c.known && now.known && c.fingerprint != now.fingerprint
}

// retryDeferred runs read. When wait is set and GitHub admission defers the
// read, it waits for an admission change, the retry boundary, or ctx, and runs
// the read again.
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
