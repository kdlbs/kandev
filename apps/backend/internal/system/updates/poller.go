package updates

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

// StartPoller launches the background polling goroutine. Calling StartPoller
// more than once without the context being cancelled is a no-op. The goroutine
// exits when ctx is cancelled; callers should retain the cancel func or rely
// on application shutdown to release it.
//
// The poller runs an immediate probe on Start so the UI can render a value
// shortly after backend boot, then ticks every PollInterval (6h by default).
// All errors are logged at warn/info and never surfaced — the next tick will
// retry. The handler never blocks on this goroutine; it serves cached values
// from kandev_meta.
func (s *Service) StartPoller(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pollerStarted {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	s.pollerStarted = true
	s.pollerCancel = cancel
	s.pollerWg.Add(1)
	interval := s.pollerInterval
	go s.pollerLoop(loopCtx, interval)
	s.log.Debug("updates poller started", zap.Duration("interval", interval))
}

// StopPoller cancels the background goroutine and waits for it to return.
// Safe to call more than once.
func (s *Service) StopPoller() {
	s.mu.Lock()
	if !s.pollerStarted {
		s.mu.Unlock()
		return
	}
	cancel := s.pollerCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.pollerWg.Wait()
	s.mu.Lock()
	s.pollerStarted = false
	s.mu.Unlock()
}

func (s *Service) pollerLoop(ctx context.Context, interval time.Duration) {
	defer s.pollerWg.Done()
	// Initial tick: run once on Start so the UI gets a value without waiting
	// the full interval after backend boot.
	s.tickOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tickOnce(ctx)
		}
	}
}

// tickOnce performs a single poll iteration. Intentionally package-internal
// so deterministic tests can drive the loop without spinning up synctest.
// GitHub rate-limit responses are logged at info; everything else at warn.
func (s *Service) tickOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if _, err := s.fetchAndPersist(ctx); err != nil {
		if errors.Is(err, ErrGitHubRateLimited) {
			s.log.Info("updates poller: github rate limited; will retry next tick")
			return
		}
		s.log.Warn("updates poller: fetch failed", zap.Error(err))
	}
}
