package linear

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotConfigured is returned when a Linear operation is attempted without a
// workspace configuration.
var ErrNotConfigured = errors.New("linear: workspace not configured")

// Client is the minimal interface the service needs from a Linear backend. The
// real implementation is GraphQLClient; tests can substitute a fake.
type Client interface {
	TestAuth(ctx context.Context) (*TestConnectionResult, error)
	GetIssue(ctx context.Context, identifier string) (*LinearIssue, error)
	ListStates(ctx context.Context, teamKey string) ([]LinearWorkflowState, error)
	SetIssueState(ctx context.Context, issueID, stateID string) error
	ListTeams(ctx context.Context) ([]LinearTeam, error)
	SearchIssues(ctx context.Context, filter SearchFilter, pageToken string, maxResults int) (*SearchResult, error)
}

// APIError captures an upstream non-2xx (or GraphQL-error) response so handlers
// can surface a meaningful status to the UI.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("linear api: status %d: %s", e.StatusCode, e.Message)
}
