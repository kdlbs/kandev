package clarification

import taskmodels "github.com/kandev/kandev/internal/task/models"

// ValidateOutcome applies the resolver's own outcome validation to an outcome
// before it is delivered, so a caller can pre-check without resolving.
func ValidateOutcome(questions []Question, o Outcome) error {
	return validateOutcome(questions, o)
}

// BundleQuestions returns a bundle's questions in the resolver's delivery
// order, with full option metadata.
func BundleQuestions(msgs []*taskmodels.Message) []Question {
	return bundleQuestions(msgs)
}

// ErrNotActive is the error IsNotActiveError recognises: the claim was lost
// and no winning resolution exists.
var ErrNotActive = errClarificationNotActive
