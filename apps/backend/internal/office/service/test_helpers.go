package service

import (
	"context"
	"fmt"
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

// GetContinuationSummaryForTest exposes the repo's continuation-summary
// read so tests can verify refreshContinuationSummary's upsert without
// duplicating the raw agent_continuation_summaries query.
func (s *Service) GetContinuationSummaryForTest(
	ctx context.Context, agentProfileID, scope string,
) (*sqlite.AgentContinuationSummary, error) {
	return s.repo.GetContinuationSummary(ctx, agentProfileID, scope)
}

// LoadContinuationSummaryForTest exposes SchedulerIntegration's private
// loadContinuationSummary so tests can drive the real reader path
// (as assembleAgentPrompt calls it) without duplicating its scope logic.
func (si *SchedulerIntegration) LoadContinuationSummaryForTest(
	ctx context.Context, run *models.Run, agentID, taskID string,
) string {
	return si.loadContinuationSummary(ctx, run, agentID, taskID)
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

// CoalesceRoutineWakeupForTest creates a wakeup-request carrying the
// given routine_id and coalesces it into an already-claimed run, exactly
// as the wakeup dispatcher does when a routine fire lands on an
// in-flight run (wakeup.Dispatcher.Dispatch -> MarkWakeupRequestCoalesced).
// Used by WO-16's continuation-scope regression tests to reproduce the
// claim-time vs completion-time snapshot drift without standing up the
// full wakeup dispatcher.
func (s *Service) CoalesceRoutineWakeupForTest(
	ctx context.Context, t *testing.T, agentProfileID, runID, routineID string,
) {
	t.Helper()
	req := &sqlite.WakeupRequest{
		ID:             "wakeup-" + runID + "-" + routineID,
		AgentProfileID: agentProfileID,
		Source:         "routine",
		Reason:         "routine_trigger",
		Payload:        fmt.Sprintf(`{"routine_id":%q}`, routineID),
	}
	if err := s.repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create wakeup request: %v", err)
	}
	if err := s.repo.MarkWakeupRequestCoalesced(ctx, req.ID, runID); err != nil {
		t.Fatalf("coalesce wakeup request into run: %v", err)
	}
}

// ExecSQL executes raw SQL against the service's database for test setup.
func (s *Service) ExecSQL(t *testing.T, query string, args ...interface{}) {
	t.Helper()
	if _, err := s.repo.ExecRaw(context.Background(), query, args...); err != nil {
		t.Fatalf("exec sql: %v", err)
	}
}

// GetTaskSessionRollupForTest reads task_sessions' AC-10 rollup columns
// directly, so tests can assert the Office cost subscriber never writes them
// (docs/specs/task-cost-ledger/spec.md AC-21).
func (s *Service) GetTaskSessionRollupForTest(t *testing.T, sessionID string) (tokensIn, tokensCachedIn, tokensOut, costSubcents int64) {
	t.Helper()
	if err := s.repo.ReaderDB().QueryRowx(
		`SELECT tokens_in, tokens_cached_in, tokens_out, cost_subcents FROM task_sessions WHERE id = ?`,
		sessionID,
	).Scan(&tokensIn, &tokensCachedIn, &tokensOut, &costSubcents); err != nil {
		t.Fatalf("read task_sessions rollup: %v", err)
	}
	return
}

// GetWorkspaceGroupForTest exposes workspace-group rows for deletion-order tests.
func (s *Service) GetWorkspaceGroupForTest(ctx context.Context, id string) (*models.WorkspaceGroup, error) {
	return s.repo.GetWorkspaceGroup(ctx, id)
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

// GetAgentRuntimeForTest exposes the repo's runtime lookup for service
// package tests asserting the last_run_finished_at stamp.
func (s *Service) GetAgentRuntimeForTest(ctx context.Context, agentID string) (*sqlite.RuntimeState, error) {
	return s.repo.GetAgentRuntime(ctx, agentID)
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

// GetWakeReceiptForTest exposes the repo's parent_child_wake_receipts read
// so ParentWakeReconciler tests can assert a paused/unresolved-assignee
// skip leaves no partial receipt behind.
func (s *Service) GetWakeReceiptForTest(ctx context.Context, parentTaskID string) (*sqlite.WakeReceipt, error) {
	return s.repo.GetWakeReceipt(ctx, parentTaskID)
}
