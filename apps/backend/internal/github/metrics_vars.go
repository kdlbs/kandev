package github

import (
	"expvar"
	"strings"
)

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler in dev mode (AC-38). Process-local and dev-mode-visible only; the
// durable, snapshot-checkable signals are the writer-health invariants
// AC-36/AC-37, not these counters. Mirrors the label idiom in
// internal/office/scheduler/metrics_vars.go.
var (
	taskPROutcomeSyncsTotal            = expvar.NewMap("github_task_pr_outcome_syncs_total")
	githubResponseClassificationsTotal = expvar.NewMap("github_provider_response_classifications_total")
	githubBackgroundDeferralsTotal     = expvar.NewMap("github_rate_limit_background_deferrals_total")
	githubSecondaryRecoveriesTotal     = expvar.NewMap("github_rate_limit_secondary_recoveries_total")
)

// outcomeMetricLabel builds a "k1=v1;k2=v2;..." label string for an expvar
// map key, matching the idiom in internal/office/scheduler/metrics_vars.go.
func outcomeMetricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// incTaskPROutcomeSync records whether a sync populated the outcome-field
// group (AC-38). populated=false is the "the writer stopped" canary's raw
// material; AC-36/AC-37 remain the durable signal.
func incTaskPROutcomeSync(populated bool) {
	taskPROutcomeSyncsTotal.Add(outcomeMetricLabel("populated", boolLabel(populated)), 1)
}

func incGitHubResponseClassification(kind FailureKind, resource Resource, retrySource RetrySource) {
	githubResponseClassificationsTotal.Add(outcomeMetricLabel(
		"kind", string(kind),
		"resource", string(resource),
		"retry_source", string(retrySource),
	), 1)
}

func incGitHubBackgroundDeferral(resource Resource, reason string) {
	githubBackgroundDeferralsTotal.Add(outcomeMetricLabel(
		"resource", string(resource), "reason", reason,
	), 1)
}

func incGitHubSecondaryRecovery(resource Resource, retrySource RetrySource, early bool) {
	githubSecondaryRecoveriesTotal.Add(outcomeMetricLabel(
		"resource", string(resource),
		"retry_source", string(retrySource),
		"early", boolLabel(early),
	), 1)
}

func boolLabel(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
