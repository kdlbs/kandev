package health

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
)

// Service runs all registered health checks.
type Service struct {
	checkers []Checker
	logger   *logger.Logger
}

// NewService creates a health service with the given checkers.
func NewService(log *logger.Logger, checkers ...Checker) *Service {
	return &Service{checkers: checkers, logger: log}
}

// RunChecks executes all checkers and returns the aggregated result.
// The Checks slice mirrors the registered checkers; each entry is marked
// passing when its checker produced no warning/error issues.
func (s *Service) RunChecks(ctx context.Context) *Response {
	issues := []Issue{}
	checks := make([]CheckSummary, 0, len(s.checkers))
	for _, c := range s.checkers {
		out := c.Check(ctx)
		issues = append(issues, out...)
		checks = append(checks, CheckSummary{
			Name:     c.Name(),
			Category: c.Category(),
			Passing:  !hasBlockingIssue(out),
		})
	}
	healthy := true
	for _, issue := range issues {
		if issue.Severity == SeverityError || issue.Severity == SeverityWarning {
			healthy = false
			break
		}
	}
	return &Response{Healthy: healthy, Issues: issues, Checks: checks}
}

// hasBlockingIssue reports whether the issue list contains any warning or
// error severity entry — info issues do not flip a check to "not passing".
func hasBlockingIssue(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError || i.Severity == SeverityWarning {
			return true
		}
	}
	return false
}
