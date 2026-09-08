package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
)

const (
	actionBudgetPolicySkipped          = "run_budget_policy_skipped"
	actionBudgetPolicyDegradedAdmitted = "run_budget_policy_degraded_admitted"
)

// utcDayStart returns the UTC midnight at or before at, the dedup boundary
// AC-OFFICE-BUDGET-002.13 and AC-OFFICE-BUDGET-004.8 share.
func utcDayStart(at time.Time) time.Time {
	u := at.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// skipIssueLabels renders a stored policy's PolicyValidationIssues as a
// stable, sorted list of short field names, so one activity entry can name
// every violation a policy has at once (AC-OFFICE-BUDGET-002.13), rather
// than one entry per violated field.
func skipIssueLabels(issues models.PolicyValidationIssues) []string {
	var out []string
	if issues.UnrecognizedPeriod {
		out = append(out, "unrecognized_period")
	}
	if issues.NonPositiveLimit {
		out = append(out, "non_positive_limit")
	}
	if issues.UnrecognizedScope {
		out = append(out, "unrecognized_scope")
	}
	if issues.UnrecognizedAction {
		out = append(out, "unrecognized_action")
	}
	if issues.EmptyScopeID {
		out = append(out, "empty_scope_id")
	}
	return out
}

// logPolicyObservability writes the operator-visible entries
// AC-OFFICE-BUDGET-002.5/-002.11/-002.14 (a stored policy this build cannot
// evaluate, skipped) and AC-OFFICE-BUDGET-004.4/-004.7/-004.8 (a policy's
// window was pricing-degraded but did not block via degradation) require,
// each deduplicated at most once per policy per UTC day
// (AC-OFFICE-BUDGET-002.13). Called once per admission evaluation of a set
// of policies (the stored policies from EvaluatePreLaunch, or a single
// built-in default result from EvaluateDefaultCeiling) independent of the
// run's final decision: AC-OFFICE-BUDGET-005.4 states the degraded-admitted
// entry is written per non-blocking-degraded policy even when a later
// policy or gate 5 still blocks the run overall — only the *counter*, not
// this entry, is conditioned on the run's final disposition, and that
// counter is not yet implemented (see task plan's deferred gaps).
func (si *SchedulerIntegration) logPolicyObservability(
	ctx context.Context, workspaceID, runID string, policies []models.PreLaunchPolicyResult, at time.Time,
) {
	day := utcDayStart(at)
	for i := range policies {
		p := &policies[i]
		switch {
		case p.Skipped:
			si.logOncePerPolicyPerDay(ctx, workspaceID, runID, actionBudgetPolicySkipped, p.PolicyID, day,
				map[string]string{
					"policy_id": p.PolicyID,
					"issues":    strings.Join(skipIssueLabels(p.SkipIssues), ","),
				})
		case p.Degraded && !p.DegradationBlocked && !p.LimitExceeded:
			fields := map[string]string{"degraded": strconv.FormatBool(true)}
			if p.IsDefault {
				fields[activityFieldCeiling] = ceilingBuiltInDefault
			} else {
				fields["policy_id"] = p.PolicyID
			}
			si.logOncePerPolicyPerDay(ctx, workspaceID, runID, actionBudgetPolicyDegradedAdmitted, p.PolicyID, day, fields)
		}
	}
}

// logOncePerPolicyPerDay writes one activity entry for (workspaceID, action,
// targetID) unless one already exists with created_at on or after day.
// Best-effort under concurrency, per AC-OFFICE-BUDGET-002.13: two runs
// evaluating the same policy concurrently may each observe no entry yet and
// each write one. A duplicate is acceptable; this bound exists to stop a
// per-evaluation stream, not to guarantee uniqueness.
func (si *SchedulerIntegration) logOncePerPolicyPerDay(
	ctx context.Context, workspaceID, runID, action, targetID string, day time.Time, fields map[string]string,
) {
	exists, err := si.svc.repo.HasActivityToday(ctx, workspaceID, action, targetID, day)
	if err != nil {
		si.logger.Warn("failed to check budget activity dedup",
			zap.String("action", action), zap.String("target_id", targetID), zap.Error(err))
		return
	}
	if exists {
		return
	}
	si.svc.LogActivityWithRun(ctx, workspaceID, "scheduler", "office-scheduler",
		action, "policy", targetID, mustJSON(fields), runID, "")
}
