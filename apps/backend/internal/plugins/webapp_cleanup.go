package plugins

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/plugins/instances"
)

const (
	webAppCleanupInterval   = time.Minute
	webAppCleanupBatch      = 32
	webAppCleanupMaxBackoff = time.Minute
)

// StartWebAppArtifactCleanupWorker starts the durable release-artifact
// cleanup loop. Jobs are claimed from SQLite before files are removed, so a
// process restart can requeue an interrupted removal without losing the
// cleanup intent.
func (s *Service) StartWebAppArtifactCleanupWorker(parent context.Context) func() {
	if s == nil || parent == nil {
		return func() {}
	}
	store := s.Instances()
	artifacts := s.WebArtifacts()
	if store == nil || artifacts == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = store.RequeueRunningCleanupJobs(ctx)
		_ = s.RunWebAppArtifactCleanupOnce(ctx)
		ticker := time.NewTicker(webAppCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.RunWebAppArtifactCleanupOnce(ctx)
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// RunWebAppArtifactCleanupOnce drains one bounded batch of due cleanup jobs.
// It is exported for startup orchestration and deterministic maintenance
// tests; normal callers should use StartWebAppArtifactCleanupWorker.
func (s *Service) RunWebAppArtifactCleanupOnce(ctx context.Context) error {
	if s == nil || ctx == nil {
		return nil
	}
	store := s.Instances()
	artifacts := s.WebArtifacts()
	if store == nil || artifacts == nil {
		return nil
	}
	for range webAppCleanupBatch {
		job, claimed, err := store.ClaimCleanupJob(ctx, time.Now().UTC())
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		_, err = store.RemoveArtifactIfUnreferenced(ctx, job.ID, job.ArtifactPath, func() error {
			return artifacts.RemoveRelativePath(job.ArtifactPath)
		})
		if err != nil {
			next := time.Now().UTC().Add(cleanupBackoff(job.Attempts))
			if retryErr := store.RetryCleanupJob(ctx, job.ID, next, err); retryErr != nil && !errors.Is(retryErr, instances.ErrNotFound) {
				return retryErr
			}
			continue
		}
	}
	return nil
}

func cleanupBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	backoff := time.Second
	for i := 1; i < attempts && backoff < webAppCleanupMaxBackoff; i++ {
		backoff *= 2
	}
	if backoff > webAppCleanupMaxBackoff {
		return webAppCleanupMaxBackoff
	}
	return backoff
}
