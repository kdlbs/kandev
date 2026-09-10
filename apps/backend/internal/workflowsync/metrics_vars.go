package workflowsync

import (
	"expvar"
	"strings"

	"github.com/kandev/kandev/internal/common/authcircuit"
)

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler in dev mode. Process-local and dev-mode-visible only; labels are
// bounded to provider name ("github"/"gitlab") and failure class
// ("transient"/"auth"/"config"), never workspace IDs, repository/project
// identifiers, branches, or error text. Mirrors the label idiom in
// internal/github/metrics_vars.go.
var (
	syncFailuresTotal = expvar.NewMap("workflowsync_failures_total")
	circuitSkipsTotal = expvar.NewMap("workflowsync_circuit_skips_total")
	circuitResetTotal = expvar.NewMap("workflowsync_circuit_resets_total")
)

// metricLabel builds a "k1=v1;k2=v2;..." label string for an expvar map
// key, matching the idiom in internal/github/metrics_vars.go.
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

// incSyncFailure records a classified sync failure, labeled only by
// provider and failure class.
func incSyncFailure(provider string, class authcircuit.FailureClass) {
	syncFailuresTotal.Add(metricLabel("provider", provider, "class", string(class)), 1)
}

// incCircuitSkip records that a due sync was skipped because its circuit
// was still open, labeled only by provider.
func incCircuitSkip(provider string) {
	circuitSkipsTotal.Add(metricLabel("provider", provider), 1)
}

// incCircuitReset records a circuit reset, labeled by provider and the
// trigger ("credential" or "config").
func incCircuitReset(provider, trigger string) {
	circuitResetTotal.Add(metricLabel("provider", provider, "trigger", trigger), 1)
}
