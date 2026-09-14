package service_test

// Covers AC-OFFICE-BUDGET-002.5/-002.11/-002.14 (a stored policy this build
// cannot evaluate is skipped, and the skip is operator-visible), and
// AC-OFFICE-BUDGET-004.4/-004.7/-004.8 (a pricing-degraded, non-blocking
// policy is operator-visible too), plus the AC-OFFICE-BUDGET-002.13
// at-most-once-per-policy-per-UTC-day dedup bound both share.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func activityDetails(t *testing.T, svc *service.Service, wsID, action string) (string, bool) {
	t.Helper()
	entries, err := svc.ListActivity(context.Background(), wsID, 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if string(e.Action) == action {
			return e.Details, true
		}
	}
	return "", false
}

func countActivityAction(t *testing.T, svc *service.Service, wsID, action string) int {
	t.Helper()
	entries, err := svc.ListActivity(context.Background(), wsID, 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	n := 0
	for _, e := range entries {
		if string(e.Action) == action {
			n++
		}
	}
	return n
}

// TestLogPolicyObservability_SkippedPolicy_NamesPolicyAndIssues covers
// AC-OFFICE-BUDGET-002.5/-002.11/-002.14: a skipped policy's entry names
// the policy and every violated field, not just one.
func TestLogPolicyObservability_SkippedPolicy_NamesPolicyAndIssues(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	policies := []models.PreLaunchPolicyResult{
		{
			PolicyID: "policy-skip-1",
			Skipped:  true,
			SkipIssues: models.PolicyValidationIssues{
				NonPositiveLimit:  true,
				UnrecognizedScope: true,
			},
		},
	}
	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-1", policies, time.Now())

	details, ok := activityDetails(t, svc, "ws-1", "run_budget_policy_skipped")
	if !ok {
		t.Fatal("expected run_budget_policy_skipped activity entry")
	}
	if !strings.Contains(details, "policy-skip-1") {
		t.Errorf("details = %q, want policy id policy-skip-1", details)
	}
	if !strings.Contains(details, "non_positive_limit") || !strings.Contains(details, "unrecognized_scope") {
		t.Errorf("details = %q, want both violated issues named", details)
	}
}

// TestLogPolicyObservability_SkippedPolicy_DedupedPerDay covers
// AC-OFFICE-BUDGET-002.13: the same policy's skip is recorded at most once
// per UTC day, not once per evaluation.
func TestLogPolicyObservability_SkippedPolicy_DedupedPerDay(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	policies := []models.PreLaunchPolicyResult{
		{PolicyID: "policy-skip-dedup", Skipped: true, SkipIssues: models.PolicyValidationIssues{NonPositiveLimit: true}},
	}
	now := time.Now()

	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-1", policies, now)
	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-2", policies, now)
	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-3", policies, now)

	if n := countActivityAction(t, svc, "ws-1", "run_budget_policy_skipped"); n != 1 {
		t.Errorf("run_budget_policy_skipped entries = %d, want exactly 1 (deduped)", n)
	}
}

// TestLogPolicyObservability_SkippedPolicy_NewDayWritesAgain covers the
// other side of AC-OFFICE-BUDGET-002.13's bound: the dedup is per UTC day,
// not forever, so a later day's evaluation writes a fresh entry. The first
// entry is backdated directly (CreateActivityEntry always stamps real
// wall-clock time, not the evaluation instant passed to
// logPolicyObservability, which is always time.Now().UTC() in production)
// to simulate "written yesterday" without waiting a real day.
func TestLogPolicyObservability_SkippedPolicy_NewDayWritesAgain(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	policies := []models.PreLaunchPolicyResult{
		{PolicyID: "policy-skip-newday", Skipped: true, SkipIssues: models.PolicyValidationIssues{NonPositiveLimit: true}},
	}
	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-1", policies, time.Now())

	svc.ExecSQL(t, `UPDATE office_activity_log SET created_at = ? WHERE action = ?`,
		time.Now().UTC().Add(-25*time.Hour), "run_budget_policy_skipped")

	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-2", policies, time.Now())

	if n := countActivityAction(t, svc, "ws-1", "run_budget_policy_skipped"); n != 2 {
		t.Errorf("run_budget_policy_skipped entries across two days = %d, want 2", n)
	}
}

// TestLogPolicyObservability_DegradedNonBlocking_WritesAdmittedEntry covers
// AC-OFFICE-BUDGET-004.4/-004.7: a policy whose window was pricing-degraded
// but which did not block (degradation exempted it, or the limit was not
// reached) gets an operator-visible, distinctly-actioned entry.
func TestLogPolicyObservability_DegradedNonBlocking_WritesAdmittedEntry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	policies := []models.PreLaunchPolicyResult{
		{PolicyID: "policy-degraded-1", Degraded: true, DegradationBlocked: false, LimitExceeded: false},
	}
	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-1", policies, time.Now())

	details, ok := activityDetails(t, svc, "ws-1", "run_budget_policy_degraded_admitted")
	if !ok {
		t.Fatal("expected run_budget_policy_degraded_admitted activity entry")
	}
	if !strings.Contains(details, "policy-degraded-1") {
		t.Errorf("details = %q, want policy id policy-degraded-1", details)
	}
}

// TestLogPolicyObservability_DegradedButLimitExceeded_NoAdmittedEntry
// covers AC-OFFICE-BUDGET-004.6: when the limit was also reached, the
// degradation fact belongs on the block entry alone (already carried by
// finishPolicyBlock's "degraded" field) -- this policy is not "admitted",
// so it must not also get a degraded-admitted entry.
func TestLogPolicyObservability_DegradedButLimitExceeded_NoAdmittedEntry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	policies := []models.PreLaunchPolicyResult{
		{PolicyID: "policy-degraded-exceeded", Degraded: true, DegradationBlocked: false, LimitExceeded: true},
	}
	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-1", policies, time.Now())

	if _, ok := activityDetails(t, svc, "ws-1", "run_budget_policy_degraded_admitted"); ok {
		t.Error("a policy whose limit was exceeded must not get a degraded-admitted entry")
	}
}

// TestLogPolicyObservability_DegradedAndBlocked_NoAdmittedEntry covers the
// symmetric case: a policy whose degradation criterion itself blocked
// (DegradationBlocked) is reported via the block entry, not this one.
func TestLogPolicyObservability_DegradedAndBlocked_NoAdmittedEntry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	policies := []models.PreLaunchPolicyResult{
		{PolicyID: "policy-degraded-blocked", Degraded: true, DegradationBlocked: true, LimitExceeded: false},
	}
	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-1", policies, time.Now())

	if _, ok := activityDetails(t, svc, "ws-1", "run_budget_policy_degraded_admitted"); ok {
		t.Error("a policy that blocked via degradation must not also get a degraded-admitted entry")
	}
}

// TestLogPolicyObservability_DefaultCeilingDegraded_NamesBuiltInDefault
// covers AC-OFFICE-BUDGET-004.8's "or stating that the ceiling was the
// built-in default" alternative to naming a policy id.
func TestLogPolicyObservability_DefaultCeilingDegraded_NamesBuiltInDefault(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	def := models.PreLaunchPolicyResult{
		IsDefault: true, Degraded: true, DegradationBlocked: false, LimitExceeded: false,
	}
	service.LogPolicyObservabilityForTest(svc, ctx, "ws-1", "run-1", []models.PreLaunchPolicyResult{def}, time.Now())

	details, ok := activityDetails(t, svc, "ws-1", "run_budget_policy_degraded_admitted")
	if !ok {
		t.Fatal("expected run_budget_policy_degraded_admitted activity entry for the built-in default")
	}
	if !strings.Contains(details, "built_in_default") {
		t.Errorf("details = %q, want the built-in default named", details)
	}
}
