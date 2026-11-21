package promptsync

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/reposync"
)

// errorClassWorkspaceMissing is the stored error class for a deleted
// credential workspace.
const errorClassWorkspaceMissing = "credential_workspace_missing"

func (s *Service) failureDirective(cfg *Config, syncErr error) reposync.FailureDirective {
	if errors.Is(syncErr, ErrCredentialWorkspaceMissing) {
		return reposync.FailureDirective{
			Class: errorClassWorkspaceMissing, Consecutive: cfg.ConsecutiveFailures + 1,
			Suspended: true, SuspensionReason: ErrCredentialWorkspaceMissing.Error(),
			CircuitClass: authcircuit.FailureClassConfig,
		}
	}
	return reposync.BuildFailureDirective(reposync.FailureInput{
		Provider:            cfg.Provider,
		IntervalSeconds:     cfg.IntervalSeconds,
		ConsecutiveFailures: cfg.ConsecutiveFailures,
		Circuit:             cfg.circuitState(),
		GenericMessage:      genericFailureMessage,
	}, syncErr, s.now().UTC(), s.jitter)
}

// ownMessages are this package's errors whose text is safe to show as is.
var ownMessages = []error{ErrCredentialWorkspaceMissing, ErrListingIncomplete}

func safeErrorMessage(err error) string {
	for _, own := range ownMessages {
		if errors.Is(err, own) {
			return own.Error()
		}
	}
	return reposync.SafeErrorMessage(err, genericFailureMessage)
}

// safeStoredMessage keeps only the exact safe summaries this package writes.
func safeStoredMessage(message string) string {
	for _, own := range ownMessages {
		if message == own.Error() {
			return message
		}
	}
	return reposync.SafeStoredErrorMessage(message, genericFailureMessage)
}

func (s *Service) recordFailure(ctx context.Context, cfg *Config, syncErr error) {
	directive := s.failureDirective(cfg, syncErr)
	incRun("failure")
	message := safeErrorMessage(syncErr)
	if err := s.store.RecordFailure(context.WithoutCancel(ctx), message, directive, s.now().UTC()); err != nil {
		s.logger.Warn("failed to record prompt sync failure", zap.Error(err))
		return
	}
	fields := []zap.Field{
		zap.String("provider", cfg.Provider),
		zap.String("failure_class", directive.Class),
		zap.Bool("poll_suspended", directive.Suspended),
		zap.String("error", message),
	}
	if directive.NextAttemptAt != nil {
		fields = append(fields, zap.Time("next_attempt_at", *directive.NextAttemptAt))
	}
	if directive.Suspended && !cfg.PollSuspended {
		s.logger.Warn("prompt sync polling suspended", fields...)
		return
	}
	s.logger.Warn("prompt sync attempt failed", fields...)
}

// recordRunError records a failed run. A background run that GitHub's local
// admission deferred did not reach the provider, so it is a scheduling wait and
// not a failure: the configuration stays due and the next tick retries it.
func (s *Service) recordRunError(ctx context.Context, cfg *Config, mode syncMode, err error) {
	var deferred *github.AdmissionDeferredError
	if mode == syncAutomatic && errors.As(err, &deferred) {
		incPollSkipped("rate_limited")
		s.logger.Info("prompt sync deferred by GitHub admission; the next tick retries",
			zap.String("provider", cfg.Provider), zap.String("reason", deferred.Reason),
			zap.Duration("delay", deferred.Delay))
		return
	}
	s.recordFailure(ctx, cfg, err)
}

func (s *Service) recordSuccess(cfg *Config, result *SyncResult) {
	incRun("success")
	if cfg.ConsecutiveFailures > 0 || cfg.PollSuspended {
		s.logger.Info("prompt sync polling recovered",
			zap.String("provider", cfg.Provider), zap.String("previous_failure_class", cfg.LastErrorClass))
	}
	s.logger.Info("applied prompt sync",
		zap.Int("created", len(result.Created)), zap.Int("updated", len(result.Updated)),
		zap.Int("deleted", len(result.Deleted)), zap.Int("warnings", len(result.Warnings)))
}
