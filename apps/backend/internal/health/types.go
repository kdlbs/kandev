package health

import "context"

// Severity indicates the importance of a health issue.
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
	SeverityInfo    Severity = "info"
)

// Issue represents a single system health issue.
type Issue struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
	FixURL   string   `json:"fix_url"`
	FixLabel string   `json:"fix_label"`
}

// Response is the API response from /api/v1/system/health.
type Response struct {
	Healthy bool    `json:"healthy"`
	Issues  []Issue `json:"issues"`
	// Checks lists every system check that ran, with a passing flag derived
	// from whether the check produced any non-info issues. Surfaced so the
	// UI can show "what's being monitored" alongside actual issues.
	Checks []CheckSummary `json:"checks"`
}

// CheckSummary describes one health checker for the UI: the user-facing name,
// the category (matches Issue.Category — useful for grouping), and a passing
// flag set to true when the check produced no warning/error issues.
type CheckSummary struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Passing  bool   `json:"passing"`
}

// Checker is the interface each health check implements. Name and Category
// are used by the service to produce the CheckSummary list in Response.
type Checker interface {
	Check(ctx context.Context) []Issue
	Name() string
	Category() string
}
