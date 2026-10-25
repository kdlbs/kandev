package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/dto"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type orphanStatusCase struct {
	name                  string
	profileID             string
	reason                string
	archive               bool
	deferred              bool
	live                  bool
	running               bool
	resumable             bool
	resumeToken           string
	wantNeedsResume       bool
	wantResumable         bool
	wantAgentRunning      bool
	wantAutoResumeAllowed bool
	wantBlockReason       string
	wantResumeReason      string
	wantError             string
}

func TestGetTaskSessionStatus_OrphanCancelledRecoveryGuards(t *testing.T) {
	for _, tc := range orphanStatusCases() {
		t.Run(tc.name, func(t *testing.T) { runOrphanStatusCase(t, tc) })
	}
}

func orphanStatusCases() []orphanStatusCase {
	return []orphanStatusCase{
		{
			name: "missing runtime row remains recoverable", profileID: "profile1",
			reason: models.SessionOrphanedCancelReason, wantNeedsResume: true,
			wantResumable: true, wantAutoResumeAllowed: true,
			wantResumeReason: resumeReasonOrphanCancelledResumable,
		},
		{
			name: "non-resumable runtime falls back to fresh start", profileID: "profile1",
			reason: models.SessionOrphanedCancelReason, running: true,
			resumeToken: "acp-session-123", wantNeedsResume: true, wantResumable: true,
			wantAutoResumeAllowed: true, wantResumeReason: resumeReasonOrphanCancelledResumable,
		},
		{
			name: "missing profile reports actionable error", reason: models.SessionOrphanedCancelReason,
			running: true, resumable: true, resumeToken: "acp-session-123",
			wantAutoResumeAllowed: true, wantError: "session missing agent profile",
		},
		{
			name: "explicit stop stays terminal", profileID: "profile1",
			reason: "user stopped the session", running: true, resumable: true,
			resumeToken: "acp-session-123", wantAutoResumeAllowed: true,
		},
		{
			name: "archived task blocks recovery", profileID: "profile1",
			reason: models.SessionOrphanedCancelReason, archive: true, running: true,
			resumable: true, resumeToken: "acp-session-123",
			wantBlockReason: resumeReasonTaskArchived, wantResumeReason: resumeReasonTaskArchived,
		},
		{
			name: "deferred owner blocks passive recovery", profileID: "profile1",
			reason: models.SessionOrphanedCancelReason, deferred: true, running: true,
			resumable: true, resumeToken: "acp-session-123", wantNeedsResume: true,
			wantResumable: true, wantBlockReason: autoResumeBlockedLaunchQueued,
			wantResumeReason: resumeReasonOrphanCancelledResumable,
		},
		{
			name: "live execution wins over recovery", profileID: "profile1",
			reason: models.SessionOrphanedCancelReason, live: true, running: true,
			resumable: true, resumeToken: "acp-session-123", wantResumable: true,
			wantAgentRunning: true, wantAutoResumeAllowed: true,
		},
	}
}

func runOrphanStatusCase(t *testing.T, tc orphanStatusCase) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCancelled)
	setOrphanStatusSession(t, repo, tc)
	if tc.deferred {
		setOrphanStatusDeferral(t, repo)
	}
	if tc.running {
		setOrphanStatusRuntime(t, repo, tc)
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	if tc.live {
		agentMgr.isAgentRunningFn = func(context.Context, string) bool { return true }
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus: %v", err)
	}
	assertOrphanStatus(t, resp, tc)
}

func setOrphanStatusSession(t *testing.T, repo *sqliterepo.Repository, tc orphanStatusCase) {
	t.Helper()
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = tc.profileID
	session.ErrorMessage = tc.reason
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	if !tc.archive {
		return
	}
	if err := repo.ArchiveTask(ctx, "task1"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	session, err = repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("reload archived session: %v", err)
	}
	session.AgentProfileID = tc.profileID
	session.ErrorMessage = tc.reason
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("restore archived session marker: %v", err)
	}
}

func setOrphanStatusDeferral(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	deferred := models.CeilingRecordKeys(models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID:      "session1",
			metaKeyAgentProfileID: "profile1",
		},
	})
	if err := repo.SetTaskMetadataKey(context.Background(), "task1", models.MetaKeyDeferredLaunch, deferred); err != nil {
		t.Fatalf("set deferred launch: %v", err)
	}
}

func setOrphanStatusRuntime(t *testing.T, repo *sqliterepo.Repository, tc orphanStatusCase) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "running1", SessionID: "session1", TaskID: "task1", Status: "ready",
		Resumable: tc.resumable, ResumeToken: tc.resumeToken, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert executor running: %v", err)
	}
}

func assertOrphanStatus(t *testing.T, resp dto.TaskSessionStatusResponse, tc orphanStatusCase) {
	t.Helper()
	if resp.NeedsResume != tc.wantNeedsResume || resp.IsResumable != tc.wantResumable {
		t.Fatalf("recovery flags = needs:%t resumable:%t, want needs:%t resumable:%t; response=%+v",
			resp.NeedsResume, resp.IsResumable, tc.wantNeedsResume, tc.wantResumable, resp)
	}
	if resp.IsAgentRunning != tc.wantAgentRunning || resp.AutoResumeAllowed != tc.wantAutoResumeAllowed {
		t.Fatalf("runtime flags = running:%t auto:%t, want running:%t auto:%t; response=%+v",
			resp.IsAgentRunning, resp.AutoResumeAllowed, tc.wantAgentRunning, tc.wantAutoResumeAllowed, resp)
	}
	if resp.AutoResumeBlockedReason != tc.wantBlockReason || resp.ResumeReason != tc.wantResumeReason {
		t.Fatalf("recovery reasons = blocked:%q resume:%q, want blocked:%q resume:%q; response=%+v",
			resp.AutoResumeBlockedReason, resp.ResumeReason, tc.wantBlockReason, tc.wantResumeReason, resp)
	}
	if resp.Error != tc.wantError {
		t.Fatalf("Error = %q, want %q; response=%+v", resp.Error, tc.wantError, resp)
	}
}

func TestGetTaskSessionStatus_OrphanCancelledStatusIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCancelled)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	session.ErrorMessage = models.SessionOrphanedCancelReason
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running1", SessionID: "session1", TaskID: "task1", Status: "ready",
		Resumable: true, ResumeToken: "acp-session-123", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert executor running: %v", err)
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	first, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("first GetTaskSessionStatus: %v", err)
	}
	second, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("second GetTaskSessionStatus: %v", err)
	}
	if first != second {
		t.Fatalf("repeated status changed response: first=%+v second=%+v", first, second)
	}
}
