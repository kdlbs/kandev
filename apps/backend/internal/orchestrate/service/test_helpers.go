package service

import (
	"context"
	"testing"
)

// BuildPromptContextForTest builds a PromptContext for testing.
// This is exposed so integration tests in the _test package can verify prompt building.
func BuildPromptContextForTest(svc *Service, ctx context.Context, reason, payload string) *PromptContext {
	si := &SchedulerIntegration{svc: svc, logger: svc.logger}
	return si.buildPromptContext(ctx, reason, payload)
}

// ExecSQL executes raw SQL against the service's database for test setup.
func (s *Service) ExecSQL(t *testing.T, query string, args ...interface{}) {
	t.Helper()
	if _, err := s.repo.ExecRaw(context.Background(), query, args...); err != nil {
		t.Fatalf("exec sql: %v", err)
	}
}
