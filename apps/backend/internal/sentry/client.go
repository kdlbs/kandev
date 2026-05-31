package sentry

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotConfigured is returned when a Sentry operation is attempted without a
// stored configuration.
var ErrNotConfigured = errors.New("sentry: not configured")

// Client is the minimal interface the service needs from a Sentry backend. The
// real implementation is RESTClient; tests can substitute a fake.
type Client interface {
	TestAuth(ctx context.Context) (*TestConnectionResult, error)
	ListOrganizations(ctx context.Context) ([]SentryOrganization, error)
	ListProjects(ctx context.Context) ([]SentryProject, error)
	SearchIssues(ctx context.Context, filter SearchFilter, cursor string) (*SearchResult, error)
	GetIssue(ctx context.Context, idOrShortID string) (*SentryIssue, error)
}

// APIError captures an upstream non-2xx response so handlers can surface a
// meaningful status to the UI.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("sentry api: status %d: %s", e.StatusCode, e.Message)
}
