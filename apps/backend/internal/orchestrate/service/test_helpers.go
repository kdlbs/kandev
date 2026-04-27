package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
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

// RunSchedulerTick runs a single scheduler tick for testing.
// This exercises the full processWakeup pipeline including task launch.
func RunSchedulerTick(svc *Service, ctx context.Context) {
	si := &SchedulerIntegration{svc: svc, logger: svc.logger}
	si.tick(ctx)
}

// BuildEnvVarsForTest exposes buildEnvVars for external test packages.
func BuildEnvVarsForTest(
	si *SchedulerIntegration,
	wakeup *models.WakeupRequest,
	agent *models.AgentInstance,
	jwt, workspaceID string,
) map[string]string {
	return si.buildEnvVars(wakeup, agent, jwt, workspaceID)
}

// GenerateSlugForTest exposes generateSlug for external test packages.
func GenerateSlugForTest(name string) string {
	return generateSlug(name)
}

// PrepareRuntimeForTest exposes prepareRuntime for external test packages.
func PrepareRuntimeForTest(
	si *SchedulerIntegration,
	ctx context.Context,
	agent *models.AgentInstance,
	workspaceSlug string,
) (string, error) {
	return si.prepareRuntime(ctx, agent, workspaceSlug)
}
