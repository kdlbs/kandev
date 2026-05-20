package scheduler

import (
	"expvar"
	"strings"
)

// expvar maps published at package init, exposed via stdlib's
// /debug/vars handler. The label model is "key=value;key=value..." so
// a Prometheus translation layer can split on `;` and `=` later
// without re-shaping the storage.
//
// Counters only — gauges that drift up and down (e.g. "currently
// degraded providers") are tricky to maintain consistently when a
// process restarts mid-degradation. A consumer that needs a snapshot
// reads the provider_health rows directly; metrics here record events.
var (
	routingFallbackTotal      = expvar.NewMap("routing_fallback_total")
	routingRouteAttemptsTotal = expvar.NewMap("routing_route_attempts_total")
	routingProviderDegraded   = expvar.NewMap("routing_provider_degraded_total")
	routingProviderRecovered  = expvar.NewMap("routing_provider_recovered_total")
	routingParkedRunsTotal    = expvar.NewMap("routing_parked_runs_total")
)

// metricLabel builds a "k1=v1;k2=v2;..." label string for an expvar
// map key. Empty values are still emitted so a downstream parser sees
// consistent cardinality dimensions. Keys are intentionally NOT
// escaped — callers control the inputs and they're alphanumeric +
// dashes (provider IDs, error codes, status names).
func metricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

func incRouteAttempt(workspaceID, providerID, outcome string) {
	routingRouteAttemptsTotal.Add(
		metricLabel("workspace", workspaceID, "provider", providerID, "outcome", outcome), 1)
}

func incRouteFallback(workspaceID, from, to, code string) {
	routingFallbackTotal.Add(
		metricLabel("workspace", workspaceID, "from", from, "to", to, "code", code), 1)
}

func incRouteParked(workspaceID, status string) {
	routingParkedRunsTotal.Add(
		metricLabel("workspace", workspaceID, "status", status), 1)
}

func incProviderDegraded(workspaceID, providerID, errorCode string) {
	routingProviderDegraded.Add(
		metricLabel("workspace", workspaceID, "provider", providerID, "code", errorCode), 1)
}

func incProviderRecovered(workspaceID, providerID string) {
	routingProviderRecovered.Add(
		metricLabel("workspace", workspaceID, "provider", providerID), 1)
}
