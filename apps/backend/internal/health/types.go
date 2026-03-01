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
}

// Checker is the interface each health check implements.
type Checker interface {
	Check(ctx context.Context) []Issue
}
