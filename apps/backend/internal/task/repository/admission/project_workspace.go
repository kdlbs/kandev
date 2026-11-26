package admission

import "context"

type projectWorkspaceValidationKey struct{}

// ProjectWorkspaceExpectation is checked again in the task insert transaction
// after the Office project was validated during request preparation.
type ProjectWorkspaceExpectation struct {
	ProjectID   string
	WorkspaceID string
}

// WithProjectWorkspaceValidation carries the admitted project relationship to
// the task repository's insert transaction.
func WithProjectWorkspaceValidation(ctx context.Context, expectation ProjectWorkspaceExpectation) context.Context {
	return context.WithValue(ctx, projectWorkspaceValidationKey{}, expectation)
}

// ProjectWorkspaceValidation returns the admitted project relationship, if present.
func ProjectWorkspaceValidation(ctx context.Context) (ProjectWorkspaceExpectation, bool) {
	expectation, ok := ctx.Value(projectWorkspaceValidationKey{}).(ProjectWorkspaceExpectation)
	return expectation, ok
}
