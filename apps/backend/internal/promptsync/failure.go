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

const quotaUnavailableMessage = "GitHub quota is temporarily unavailable"

func (s *Service) failureDirective(cfg *Config, syncErr error) reposync.FailureDirective {
	if errors.Is(syncErr, ErrCredentialWorkspaceMissing) {
		return reposync.FailureDirective{
			Class: errorClassWorkspaceMissing, Consecutive: cfg.ConsecutiveFailures + 1,
			Suspended: true, SuspensionReason: ErrCredentialWorkspaceMissing.Error(),
			CircuitClass: authcircuit.FailureClassConfig,
		}
	}
	directive := reposync.BuildFailureDirective(reposync.FailureInput{
		Provider:            cfg.Provider,
		IntervalSeconds:     cfg.IntervalSeconds,
		ConsecutiveFailures: cfg.ConsecutiveFailures,
		Circuit:             cfg.circuitState(),
		GenericMessage:      genericFailureMessage,
	}, syncErr, s.now().UTC(), s.jitter)
	if errors.Is(syncErr, github.ErrGitHubAdmissionWaitRequired) {
		directive.Class = github.RateLimitErrorCode
	}
	return directive
}

// ownMessages are this package's errors whose text is safe to show as is.
var ownMessages = []error{ErrCredentialWorkspaceMissing, ErrListingIncomplete}

func safeErrorMessage(err error) string {
	if errors.Is(err, github.ErrGitHubAdmissionWaitRequired) {
		return quotaUnavailableMessage
	}
	for _, own := range ownMessages {
		if errors.Is(err, own) {
			return own.Error()
		}
	}
	return reposync.SafeErrorMessage(err, genericFailureMessage)
}

// safeStoredMessage keeps only the exact safe summaries this package writes.
func safeStoredMessage(message string) string {
	if message == quotaUnavailableMessage {
		return message
	}
	for _, own := range ownMessages {
		if message == own.Error() {
			return message
		}
	}
	return reposync.SafeStoredErrorMessage(message, genericFailureMessage)
}

func (s *Service) recordFailure(ctx context.Context, cfg *Config, mode syncMode, syncErr error) {
	directive := s.failureDirective(cfg, syncErr)
	message := safeErrorMessage(syncErr)
	// Manual failure status survives request cancellation. Automatic status
	// writes follow the poller's cancelable run context.
	writeCtx := context.WithoutCancel(ctx)
	if mode == syncAutomatic {
		writeCtx = ctx
	}
	if err := s.store.RecordFailure(writeCtx, message, directive, s.now().UTC()); err != nil {
		if mode == syncAutomatic && ctx.Err() != nil {
			return
		}
		s.logger.Warn("failed to record prompt sync failure", zap.Error(err))
		return
	}
	incRun("failure")
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
// admission deferred did not reach the provider, so it leaves the configuration
// due without recording a failure.
func (s *Service) recordRunError(ctx context.Context, cfg *Config, mode syncMode, err error) {
	var deferred *github.AdmissionDeferredError
	if mode == syncAutomatic && errors.As(err, &deferred) {
		return
	}
	s.recordFailure(ctx, cfg, mode, err)
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
