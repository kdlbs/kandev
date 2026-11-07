package github

import (
	"expvar"
	"strings"
)

const (
	reviewCleanupMetricClassAuth      = "auth"
	reviewCleanupMetricClassOther     = "other"
	reviewCleanupMetricClassRateLimit = "rate_limit"
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
	// authCircuitSkipsTotal and authCircuitResetsTotal are unlabeled counts
	// (no per-workspace label — workspace IDs must never be metric labels).
	// They bound the PR-monitor loop's auth/config circuit-breaker activity
	// (see poller_circuit.go): skips confirm a broken credential/config
	// stopped costing a GitHub call every cycle; resets confirm a
	// credential change (rotate/reconnect) is promptly detected.
	authCircuitSkipsTotal            = expvar.NewInt("github_pr_watch_auth_circuit_skips_total")
	authCircuitResetsTotal           = expvar.NewInt("github_pr_watch_auth_circuit_resets_total")
	prWatchActive                    = expvar.NewInt("github_pr_watch_active")
	prWatchSearching                 = expvar.NewInt("github_pr_watch_searching")
	prWatchDuplicates                = expvar.NewInt("github_pr_watch_duplicates")
	prWatchOrphans                   = expvar.NewInt("github_pr_watch_orphans")
	canonicalPollRequests            = expvar.NewInt("github_pr_watch_canonical_poll_requests_total")
	reviewCleanupCircuitSkipsTotal   = expvar.NewMap("github_review_cleanup_circuit_skips_total")
	reviewCleanupCircuitResetsTotal  = expvar.NewMap("github_review_cleanup_circuit_resets_total")
	reviewCleanupFailuresTotal       = expvar.NewMap("github_review_cleanup_failures_total")
	reviewCleanupCoreQuotaSkipsTotal = expvar.NewInt("github_review_cleanup_core_quota_skips_total")
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

// incAuthCircuitSkip records that a poll cycle skipped a workspace's PR
// watches entirely because its auth/config circuit was still open.
func incAuthCircuitSkip() {
	authCircuitSkipsTotal.Add(1)
}

// incAuthCircuitReset records a credential-fingerprint-triggered circuit
// reset (rotate/reconnect/revoke-then-reconfigure detected).
func incAuthCircuitReset() {
	authCircuitResetsTotal.Add(1)
}

func incReviewCleanupCircuitSkip(scope, class string) {
	if !validReviewCleanupMetricScope(scope) {
		return
	}
	reviewCleanupCircuitSkipsTotal.Add(outcomeMetricLabel("scope", scope, "class", boundedReviewCleanupMetricClass(class)), 1)
}

func incReviewCleanupCircuitReset(scope string) {
	if validReviewCleanupMetricScope(scope) {
		reviewCleanupCircuitResetsTotal.Add(outcomeMetricLabel("scope", scope), 1)
	}
}

func incReviewCleanupFailure(scope, class string) {
	if !validReviewCleanupMetricScope(scope) {
		return
	}
	reviewCleanupFailuresTotal.Add(outcomeMetricLabel("scope", scope, "class", boundedReviewCleanupMetricClass(class)), 1)
}

func incReviewCleanupCoreQuotaSkip() {
	reviewCleanupCoreQuotaSkipsTotal.Add(1)
}

func validReviewCleanupMetricScope(scope string) bool {
	return scope == "workspace" || scope == "record"
}

func boundedReviewCleanupMetricClass(class string) string {
	switch class {
	case reviewCleanupMetricClassAuth, "config", "transient", reviewCleanupMetricClassRateLimit:
		return class
	default:
		return reviewCleanupMetricClassOther
	}
}

func recordPRWatchCardinality(cardinality PRWatchCardinality) {
	prWatchActive.Set(cardinality.Active)
	prWatchSearching.Set(cardinality.Searching)
	prWatchDuplicates.Set(cardinality.Duplicates)
	prWatchOrphans.Set(cardinality.Orphans)
}

func incCanonicalPollRequests(count int) {
	canonicalPollRequests.Add(int64(count))
}
