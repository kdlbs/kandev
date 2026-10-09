package orchestrator

import (
	"go.uber.org/zap"
)

// unlimitedSessionCeiling is the configured value that disables refusal, matching
// the wip_limit convention.
const unlimitedSessionCeiling = 0

// newSessionCeilingForRepo builds the controller the service owns, binding the
// repository as the persisted half of its population source.
//
// The repository is reached through a narrow consumer-side interface and a type
// assertion, following the idiom the other repository consumers in this package
// use. That idiom degrades silently, and here the degraded state is dangerous
// rather than merely reduced: an unbound controller still answers, but it counts
// only its own in-process reservations, so every session already running on the
// instance is invisible to it and the ceiling admits without bound. A build-time
// assertion covers the production repository; this warning covers everything else.
func newSessionCeilingForRepo(repo interface{}, ceiling int, logger *zap.Logger) *sessionCeilingController {
	if logger == nil {
		logger = zap.NewNop()
	}
	lister, ok := repo.(admittedSessionLister)
	if !ok {
		logger.Warn("repository cannot enumerate admitted sessions; the session ceiling will count only in-process reservations")
		lister = nil
	}
	return newSessionCeilingController(ceiling, lister, logger)
}
