package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// ListRunEventsForTest exposes the repo's ListRunEvents query so the
// run-lifecycle integration tests can verify per-run event emission.
func (s *Service) ListRunEventsForTest(
	ctx context.Context, runID string,
) ([]*models.RunEvent, error) {
	return s.repo.ListRunEvents(ctx, runID, -1, 0)
}

// ListTasksTouchedByRunForTest exposes the repo's read query so the
// run-lifecycle integration tests can verify run_id plumbing on the
// activity log.
func (s *Service) ListTasksTouchedByRunForTest(
	ctx context.Context, runID string,
) ([]string, error) {
	return s.repo.ListTasksTouchedByRun(ctx, runID)
}

// ListRunSkillSnapshotsForTest exposes skill snapshots captured at launch.
func (s *Service) ListRunSkillSnapshotsForTest(
	ctx context.Context, runID string,
) ([]models.RunSkillSnapshot, error) {
	return s.repo.ListRunSkillSnapshots(ctx, runID)
}

// IsRateLimitErrorForTest exposes isRateLimitError for external test packages.
func IsRateLimitErrorForTest(errMsg string) bool {
	return isRateLimitError(errMsg)
}

// ParseRateLimitResetTimeForTest exposes parseRateLimitResetTime for external test packages.
func ParseRateLimitResetTimeForTest(errMsg string, now time.Time) *time.Time {
	return parseRateLimitResetTime(errMsg, now)
}

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

// GetTaskExecutionFieldsForTest exposes task execution fields for service package tests.
func (s *Service) GetTaskExecutionFieldsForTest(
	ctx context.Context,
	taskID string,
) (*sqlite.TaskExecutionFields, error) {
	return s.repo.GetTaskExecutionFields(ctx, taskID)
}

// GetTaskAssigneeForTest exposes the task's assigned agent instance id
// for service package tests. Used by the queueRunDispatcher test
// helper to mimic the engine's queue_run target resolution.
func (s *Service) GetTaskAssigneeForTest(ctx context.Context, taskID string) (string, error) {
	return s.repo.GetTaskAssignee(ctx, taskID)
}

// RunSchedulerTick runs a single scheduler tick for testing.
// This exercises the full processRun pipeline including task launch.
func RunSchedulerTick(svc *Service, ctx context.Context) {
	si := &SchedulerIntegration{svc: svc, logger: svc.logger}
	si.tick(ctx)
}

// BuildEnvVarsForTest exposes buildEnvVars for external test packages.
func BuildEnvVarsForTest(
	si *SchedulerIntegration,
	run *models.Run,
	agent *models.AgentInstance,
	jwt, workspaceID string,
) map[string]string {
	return si.buildEnvVars(run, agent, jwt, workspaceID)
}

// GenerateSlugForTest exposes generateSlug for external test packages.
func GenerateSlugForTest(name string) string {
	return generateSlug(name)
}

// PrepareRuntimeForTest was removed in ADR 0005 Wave E with
// SchedulerIntegration.prepareRuntime; runtime export is now owned by
// internal/agent/runtime/lifecycle/skill.

// BuildSkillManifestForTest exposes buildSkillManifest for external test packages.
func BuildSkillManifestForTest(
	si *SchedulerIntegration,
	ctx context.Context,
	agent *models.AgentInstance,
	workspaceSlug string,
) *SkillManifest {
	return si.buildSkillManifest(ctx, agent, workspaceSlug)
}

// Skill delivery test helpers were removed in ADR 0005 Wave E along
// with the office-tier delivery code. Coverage moved into
// internal/agent/runtime/lifecycle/skill.
