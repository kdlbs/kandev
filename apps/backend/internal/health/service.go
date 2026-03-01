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
func (s *Service) RunChecks(ctx context.Context) *Response {
	var issues []Issue
	for _, c := range s.checkers {
		issues = append(issues, c.Check(ctx)...)
	}
	healthy := true
	for _, issue := range issues {
		if issue.Severity == SeverityError || issue.Severity == SeverityWarning {
			healthy = false
			break
		}
	}
	if issues == nil {
		issues = []Issue{}
	}
	return &Response{Healthy: healthy, Issues: issues}
}
