package costs

import (
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
)

// ErrInvalidBudgetPolicy wraps every rejection validateBudgetPolicyWrite
// returns, so a caller can distinguish "the write was malformed" from any
// other failure (e.g. a repository error) with errors.Is.
var ErrInvalidBudgetPolicy = errors.New("invalid budget policy")

// validateBudgetPolicyWrite implements AC-OFFICE-BUDGET-002.6/.7: reject a
// create or update before it reaches storage, rather than persisting a
// policy that blocks every run (non-positive limit) or matches every run
// whose corresponding identifier is also empty (agent/project scope with no
// scope_id).
func validateBudgetPolicyWrite(policy *models.BudgetPolicy) error {
	if policy.LimitSubcents <= 0 {
		return fmt.Errorf("%w: limit_subcents must be positive, got %d", ErrInvalidBudgetPolicy, policy.LimitSubcents)
	}
	if (policy.ScopeType == scopeAgent || policy.ScopeType == scopeProject) && policy.ScopeID == "" {
		return fmt.Errorf("%w: scope_id is required for scope %q", ErrInvalidBudgetPolicy, policy.ScopeType)
	}
	return nil
}
