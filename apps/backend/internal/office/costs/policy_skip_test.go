package costs

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestClassifyStoredPolicy pins AC-OFFICE-BUDGET-002.5/.11/.14: a stored
// policy the build cannot evaluate as written is skipped, never treated as
// a lifetime window or as matching every run. Every violation on the row is
// detected in the same pass (AC-OFFICE-BUDGET-002.13 names all of them in
// one entry, so the classifier must not stop at the first).
func TestClassifyStoredPolicy(t *testing.T) {
	validPolicy := func() *models.BudgetPolicy {
		return &models.BudgetPolicy{
			ScopeType:      models.BudgetScopeWorkspace,
			LimitSubcents:  1000,
			Period:         models.BudgetPeriodDaily,
			ActionOnExceed: models.BudgetActionNotifyOnly,
		}
	}

	tests := []struct {
		name   string
		policy func() *models.BudgetPolicy
		want   policyValidationIssues
	}{
		{
			name:   "fully valid workspace policy has no issues",
			policy: validPolicy,
			want:   policyValidationIssues{},
		},
		{
			name: "unrecognized period",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.Period = models.BudgetPeriod("weekly")
				return p
			},
			want: policyValidationIssues{UnrecognizedPeriod: true},
		},
		{
			name: "non-positive limit",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.LimitSubcents = 0
				return p
			},
			want: policyValidationIssues{NonPositiveLimit: true},
		},
		{
			name: "unrecognized scope type",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.ScopeType = models.BudgetScopeType("team")
				return p
			},
			want: policyValidationIssues{UnrecognizedScope: true},
		},
		{
			name: "unrecognized action",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.ActionOnExceed = models.BudgetActionOnExceed("delete_everything")
				return p
			},
			want: policyValidationIssues{UnrecognizedAction: true},
		},
		{
			name: "agent scope with empty scope_id",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.ScopeType = models.BudgetScopeAgent
				p.ScopeID = ""
				return p
			},
			want: policyValidationIssues{EmptyScopeID: true},
		},
		{
			name: "project scope with empty scope_id",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.ScopeType = models.BudgetScopeProject
				p.ScopeID = ""
				return p
			},
			want: policyValidationIssues{EmptyScopeID: true},
		},
		{
			name: "agent scope with a scope_id has no issues",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.ScopeType = models.BudgetScopeAgent
				p.ScopeID = "agent-1"
				return p
			},
			want: policyValidationIssues{},
		},
		{
			name: "workspace scope never needs a scope_id",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.ScopeID = ""
				return p
			},
			want: policyValidationIssues{},
		},
		{
			name: "every violation is detected in one pass, not just the first",
			policy: func() *models.BudgetPolicy {
				p := validPolicy()
				p.Period = models.BudgetPeriod("weekly")
				p.LimitSubcents = -5
				p.ScopeType = models.BudgetScopeType("team")
				p.ActionOnExceed = models.BudgetActionOnExceed("nope")
				return p
			},
			want: policyValidationIssues{
				UnrecognizedPeriod: true,
				NonPositiveLimit:   true,
				UnrecognizedScope:  true,
				UnrecognizedAction: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyStoredPolicy(tt.policy())
			if got != tt.want {
				t.Errorf("classifyStoredPolicy() = %+v, want %+v", got, tt.want)
			}
			wantAny := tt.want != (policyValidationIssues{})
			if got.Any() != wantAny {
				t.Errorf("classifyStoredPolicy().Any() = %v, want %v", got.Any(), wantAny)
			}
		})
	}
}
