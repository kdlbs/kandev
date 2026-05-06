package scheduler

import (
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Routing metric event names. They live under the routing.metric.*
// namespace so future log-aggregation rules can match on the message
// without scanning a free-form string. If a real metrics backend is
// introduced later, these names map 1:1 to Prometheus counter/gauge
// names — the field set is already shaped for that translation.
const (
	metricRouteAttempt      = "routing.metric.route_attempt"
	metricRouteFallback     = "routing.metric.route_fallback"
	metricRouteParked       = "routing.metric.route_parked"
	metricProviderDegraded  = "routing.metric.provider_degraded"
	metricProviderRecovered = "routing.metric.provider_recovered"
)

// Route attempt outcomes used by the metric.route_attempt event. Kept
// distinct from RouteAttemptOutcome* so the metric channel does not
// drift if those internal names change.
const (
	metricOutcomeSuccess        = "success"
	metricOutcomeFallbackErr    = "fallback_allowed_error"
	metricOutcomeFatalErr       = "fatal_error"
	metricOutcomeSkipped        = "skipped"
	metricOutcomeParkedNoCandid = "parked_no_candidates"
)

// recordRouteAttempt emits a structured log event for one route-attempt
// outcome AND increments the expvar counter under
// `routing_route_attempts_total`. Workspace + provider + outcome are
// the cardinality dimensions.
func (ss *SchedulerService) recordRouteAttempt(
	workspaceID, providerID, outcome, errorCode string,
) {
	incRouteAttempt(workspaceID, providerID, outcome)
	if ss.logger == nil {
		return
	}
	ss.logger.Info(metricRouteAttempt,
		zap.String("workspace_id", workspaceID),
		zap.String("provider_id", providerID),
		zap.String("outcome", outcome),
		zap.String("error_code", errorCode))
}

// recordRouteFallback emits a structured log AND bumps the expvar
// counter under `routing_fallback_total` when the dispatcher walks
// from one candidate to another.
func (ss *SchedulerService) recordRouteFallback(
	workspaceID, fromProvider, toProvider, errorCode string,
) {
	incRouteFallback(workspaceID, fromProvider, toProvider, errorCode)
	if ss.logger == nil {
		return
	}
	ss.logger.Info(metricRouteFallback,
		zap.String("workspace_id", workspaceID),
		zap.String("from_provider", fromProvider),
		zap.String("to_provider", toProvider),
		zap.String("error_code", errorCode))
}

// recordRouteParked emits a structured log AND bumps the expvar
// counter under `routing_parked_runs_total` when a run gets parked
// under a routing_blocked_status.
func (ss *SchedulerService) recordRouteParked(
	workspaceID, runID, status string,
) {
	incRouteParked(workspaceID, status)
	if ss.logger == nil {
		return
	}
	ss.logger.Info(metricRouteParked,
		zap.String("workspace_id", workspaceID),
		zap.String("run_id", runID),
		zap.String("status", status))
}

// recordProviderDegraded emits a structured log AND bumps the expvar
// counter under `routing_provider_degraded_total` when a provider
// health row flips degraded.
func recordProviderDegraded(
	log *logger.Logger, workspaceID, providerID, errorCode string,
) {
	incProviderDegraded(workspaceID, providerID, errorCode)
	if log == nil {
		return
	}
	log.Info(metricProviderDegraded,
		zap.String("workspace_id", workspaceID),
		zap.String("provider_id", providerID),
		zap.String("error_code", errorCode))
}

// recordProviderRecovered emits a structured log AND bumps the expvar
// counter under `routing_provider_recovered_total` when a provider
// health row flips back to healthy. Paired with recordProviderDegraded
// so a metrics scraper can compute the current "currently degraded"
// gauge from the diff of the two counters.
func recordProviderRecovered(
	log *logger.Logger, workspaceID, providerID string,
) {
	incProviderRecovered(workspaceID, providerID)
	if log == nil {
		return
	}
	log.Info(metricProviderRecovered,
		zap.String("workspace_id", workspaceID),
		zap.String("provider_id", providerID))
}
