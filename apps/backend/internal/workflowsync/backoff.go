package workflowsync

import (
	"time"

	"github.com/kandev/kandev/internal/reposync"
)

type failureDirective = reposync.FailureDirective

func buildFailureDirective(
	cfg *Config,
	syncErr error,
	now time.Time,
	jitter func(time.Duration) time.Duration,
) failureDirective {
	return reposync.BuildFailureDirective(reposync.FailureInput{
		Provider:            cfg.Provider,
		IntervalSeconds:     cfg.IntervalSeconds,
		ConsecutiveFailures: cfg.ConsecutiveFailures,
		Circuit:             cfg.circuitState(),
		GenericMessage:      genericSyncFailureMessage,
	}, syncErr, now, jitter)
}
