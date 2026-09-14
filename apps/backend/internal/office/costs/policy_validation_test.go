package costs

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestValidateBudgetPolicyWrite pins AC-OFFICE-BUDGET-002.6/.7: create/update
// rejects a non-positive limit and an agent/project scope with an empty
// scope identifier, rather than storing a policy that blocks every run or
// matches every run whose corresponding identifier is also empty.
func TestValidateBudgetPolicyWrite(t *testing.T) {
	validBase := func() *models.BudgetPolicy {
		return &models.BudgetPolicy{
			WorkspaceID:   "ws-1",
			ScopeType:     models.BudgetScopeType(scopeWorkspace),
			LimitSubcents: 1000,
			Period:        models.BudgetPeriodDaily,
		}
	}

	tests := []struct {
		name    string
		policy  func() *models.BudgetPolicy
		wantErr bool
	}{
		{
			name:    "valid workspace-scoped policy is accepted",
			policy:  validBase,
			wantErr: false,
		},
		{
			name: "zero limit is rejected",
			policy: func() *models.BudgetPolicy {
				p := validBase()
				p.LimitSubcents = 0
				return p
			},
			wantErr: true,
		},
		{
			name: "negative limit is rejected",
			policy: func() *models.BudgetPolicy {
				p := validBase()
				p.LimitSubcents = -1
				return p
			},
			wantErr: true,
		},
		{
			name: "agent scope with empty scope_id is rejected",
			policy: func() *models.BudgetPolicy {
				p := validBase()
				p.ScopeType = models.BudgetScopeType(scopeAgent)
				p.ScopeID = ""
				return p
			},
			wantErr: true,
		},
		{
			name: "project scope with empty scope_id is rejected",
			policy: func() *models.BudgetPolicy {
				p := validBase()
				p.ScopeType = models.BudgetScopeType(scopeProject)
				p.ScopeID = ""
				return p
			},
			wantErr: true,
		},
		{
			name: "agent scope with a scope_id is accepted",
			policy: func() *models.BudgetPolicy {
				p := validBase()
				p.ScopeType = models.BudgetScopeType(scopeAgent)
				p.ScopeID = "agent-1"
				return p
			},
			wantErr: false,
		},
		{
			name: "workspace scope never needs a scope_id",
			policy: func() *models.BudgetPolicy {
				p := validBase()
				p.ScopeID = ""
				return p
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBudgetPolicyWrite(tt.policy())
			if tt.wantErr && err == nil {
				t.Fatal("validateBudgetPolicyWrite() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateBudgetPolicyWrite() = %v, want nil", err)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidBudgetPolicy) {
				t.Errorf("validateBudgetPolicyWrite() = %v, want errors.Is(err, ErrInvalidBudgetPolicy)", err)
			}
		})
	}
}
