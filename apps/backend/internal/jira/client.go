package jira

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotConfigured is returned when a Jira operation is attempted without a
// workspace configuration.
var ErrNotConfigured = errors.New("jira: workspace not configured")

// Client is the minimal interface service needs from a Jira backend. The real
// implementation is CloudClient; tests can substitute a fake.
type Client interface {
	TestAuth(ctx context.Context) (*TestConnectionResult, error)
	GetTicket(ctx context.Context, ticketKey string) (*JiraTicket, error)
	ListTransitions(ctx context.Context, ticketKey string) ([]JiraTransition, error)
	DoTransition(ctx context.Context, ticketKey, transitionID string) error
	ListProjects(ctx context.Context) ([]JiraProject, error)
	SearchTickets(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error)
}

// APIError captures an upstream non-2xx response so handlers can surface a
// meaningful status to the UI.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jira api: status %d: %s", e.StatusCode, e.Message)
}
