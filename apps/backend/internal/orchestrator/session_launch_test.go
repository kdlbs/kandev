package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type rejectingLaunchAttachmentClaimer struct {
	wantErr     error
	taskID      string
	sessionID   string
	attachments []v1.MessageAttachment
}

func (c *rejectingLaunchAttachmentClaimer) ClaimMessageAttachments(
	_ context.Context,
	taskID, sessionID string,
	attachments []v1.MessageAttachment,
) error {
	c.taskID = taskID
	c.sessionID = sessionID
	c.attachments = attachments
	return c.wantErr
}

func TestExecutionToLaunchResponseReportsAgentProfile(t *testing.T) {
	response := executionToLaunchResponse("task-1", &executor.TaskExecution{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		AgentProfileID:   "effective-profile",
		SessionState:     v1.TaskSessionStateRunning,
	})
	if got, want := response.AgentProfileID, "effective-profile"; got != want {
		t.Fatalf("agent_profile_id = %v, want %v", got, want)
	}
}

func TestExecutionToLaunchResponseHandlesNilExecution(t *testing.T) {
	// A guarded automatic launch can return a successful no-op without an
	// execution. The response helper is defensive for callers that use it
	// directly or through another launch path.
	response := executionToLaunchResponse("task-1", nil)
	if response == nil {
		t.Fatal("expected a response")
		return
	}
	if !response.Success {
		t.Fatal("expected a successful no-op response")
	}
	if response.TaskID != "task-1" {
		t.Fatalf("task_id = %q, want task-1", response.TaskID)
	}
	if response.SessionID != "" {
		t.Fatalf("session_id = %q, want empty", response.SessionID)
	}
}

func TestResolveIntent(t *testing.T) {
	tests := []struct {
		name string
		req  LaunchSessionRequest
		want SessionIntent
	}{
		// Explicit intents take priority
		{
			name: "explicit start intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentStart},
			want: IntentStart,
		},
		{
			name: "explicit resume intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentResume, SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "explicit prepare intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentPrepare},
			want: IntentPrepare,
		},
		{
			name: "explicit start_created intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentStartCreated, SessionID: "s1"},
			want: IntentStartCreated,
		},
		{
			name: "explicit workflow_step intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentWorkflowStep, SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},

		// Inferred intents (no explicit intent set)
		{
			name: "workflow_step inferred from session_id + workflow_step_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},
		{
			name: "resume inferred from session_id only",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "resume inferred from session_id with no prompt and no agent_profile",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "start_created inferred from session_id + prompt",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", Prompt: "hello"},
			want: IntentStartCreated,
		},
		{
			name: "start_created inferred from session_id + agent_profile_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", AgentProfileID: "ap1"},
			want: IntentStartCreated,
		},
		{
			name: "start_created inferred from session_id + prompt + agent_profile_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", Prompt: "hello", AgentProfileID: "ap1"},
			want: IntentStartCreated,
		},
		{
			name: "prepare inferred from launch_workspace without prompt",
			req:  LaunchSessionRequest{TaskID: "t1", LaunchWorkspace: true},
			want: IntentPrepare,
		},
		{
			name: "start inferred from minimal request",
			req:  LaunchSessionRequest{TaskID: "t1"},
			want: IntentStart,
		},
		{
			name: "start inferred when prompt provided without session_id",
			req:  LaunchSessionRequest{TaskID: "t1", Prompt: "do something"},
			want: IntentStart,
		},
		{
			name: "start inferred when launch_workspace + prompt (not prepare)",
			req:  LaunchSessionRequest{TaskID: "t1", LaunchWorkspace: true, Prompt: "do something"},
			want: IntentStart,
		},

		// Edge cases
		{
			name: "resume wins over start_created when session_id set, no prompt, no agent_profile",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", ExecutorID: "e1"},
			want: IntentResume,
		},
		{
			name: "workflow_step wins over resume when both session_id and workflow_step_id set",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveIntent(&tt.req)
			if got != tt.want {
				t.Errorf("ResolveIntent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLaunchSession_RejectsAttachmentClaimBeforeStart(t *testing.T) {
	wantErr := errors.New("attachment is not available")
	claimer := &rejectingLaunchAttachmentClaimer{wantErr: wantErr}
	service := &Service{}
	service.SetLaunchAttachmentClaimer(claimer)

	attachment := v1.MessageAttachment{AttachmentID: "attachment-1", Name: "notes.txt"}
	_, err := service.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID:      "task-1",
		SessionID:   "session-1",
		Intent:      IntentStartCreated,
		Prompt:      "read the attachment",
		Attachments: []v1.MessageAttachment{attachment},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("LaunchSession error = %v, want %v", err, wantErr)
	}
	if claimer.taskID != "task-1" || claimer.sessionID != "session-1" {
		t.Fatalf("claim scope = (%q, %q), want (task-1, session-1)", claimer.taskID, claimer.sessionID)
	}
	if len(claimer.attachments) != 1 || claimer.attachments[0].AttachmentID != attachment.AttachmentID {
		t.Fatalf("claimed attachments = %+v", claimer.attachments)
	}
}

func TestLaunchSession_RejectsMismatchedTaskSessionBeforeAttachmentClaim(t *testing.T) {
	claimer := &rejectingLaunchAttachmentClaimer{wantErr: errors.New("claimer must not run")}
	service := &Service{repo: &pairStubRepo{sessionTaskID: "task-other"}}
	service.SetLaunchAttachmentClaimer(claimer)

	_, err := service.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID:      "task-1",
		SessionID:   "session-1",
		Intent:      IntentStartCreated,
		Prompt:      "read the attachment",
		Attachments: []v1.MessageAttachment{{AttachmentID: "attachment-1"}},
	})
	if err == nil || !strings.Contains(err.Error(), "does not belong to task") {
		t.Fatalf("LaunchSession error = %v, want task/session mismatch", err)
	}
	if claimer.taskID != "" || len(claimer.attachments) != 0 {
		t.Fatalf("attachment claimer ran for mismatched pair: %+v", claimer)
	}
}

func TestNormalizeRecoverSessionError(t *testing.T) {
	t.Run("maps profile not found errors to actionable profile guidance", func(t *testing.T) {
		in := errors.New("failed to resolve agent profile: profile not found: sql: no rows in result set")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected mapped error")
		}
		want := "the agent profile used by this session was deleted; start a new session and choose an available agent profile: " + in.Error()
		if got := err.Error(); got != want {
			t.Fatalf("unexpected error: %q", got)
		}
	})

	t.Run("maps agent profile not found errors to actionable profile guidance", func(t *testing.T) {
		in := errors.New("agent profile not found")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected mapped error")
		}
		want := "the agent profile used by this session was deleted; start a new session and choose an available agent profile: " + in.Error()
		if got := err.Error(); got != want {
			t.Fatalf("unexpected error: %q", got)
		}
	})

	t.Run("does not map generic sql no rows errors", func(t *testing.T) {
		in := errors.New("sql: no rows in result set")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected passthrough error")
		}
		if err.Error() != in.Error() {
			t.Fatalf("expected passthrough error %q, got %q", in.Error(), err.Error())
		}
	})

	t.Run("does not map executor profile not found errors", func(t *testing.T) {
		in := errors.New("executor profile not found")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected passthrough error")
		}
		if err.Error() != in.Error() {
			t.Fatalf("expected passthrough error %q, got %q", in.Error(), err.Error())
		}
	})

	t.Run("passes through unrelated errors", func(t *testing.T) {
		in := errors.New("network timeout")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected passthrough error")
		}
		if err.Error() != in.Error() {
			t.Fatalf("expected passthrough error %q, got %q", in.Error(), err.Error())
		}
	})
}

func TestRecoverSession_OfficeResolvesBlockForSchedulerAdmission(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	seedTaskAndSession(t, repo, "task-office-recovery", "session-office-recovery", models.TaskSessionStateFailed)

	task, err := repo.GetTask(ctx, "task-office-recovery")
	if err != nil {
		t.Fatalf("load Office task: %v", err)
	}
	// IsFromOffice is a read-time projection. Mark the persisted task with an
	// Office project so lookupOfficeTask observes the same production identity
	// that the scheduler uses.
	task.ProjectID = "project-office-recovery"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("mark task as Office-owned: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-office-recovery")
	if err != nil {
		t.Fatalf("load Office session: %v", err)
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}

	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID:                 "office-recovery-block",
		SessionID:          "session-office-recovery",
		IncarnationID:      incarnationID,
		ExpectedGeneration: 0,
		Reason:             "native_state_missing",
		State:              models.RecoveryBlockOpen,
	}); err != nil {
		t.Fatalf("persist Office recovery block: %v", err)
	}

	response, err := svc.RecoverSession(ctx, "task-office-recovery", "session-office-recovery", "continue_from_history")
	if err != nil {
		t.Fatalf("RecoverSession: %v", err)
	}
	if response == nil || !response.Success {
		t.Fatalf("response = %+v, want successful scheduler authorization", response)
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-office-recovery"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("Office recovery launched a direct executor, err = %v", err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "office-recovery-block")
	if err != nil {
		t.Fatalf("load resolved Office recovery block: %v", err)
	}
	if block.State != models.RecoveryBlockResolved || block.AuthorizedAction != "continue_from_history" {
		t.Fatalf("resolved Office recovery block = %+v", block)
	}
}

type continuationAdmissionAgentManager struct {
	*mockAgentManager
	repo      *sqliterepo.Repository
	taskID    string
	sessionID string
}

func (m *continuationAdmissionAgentManager) StartAgentProcess(ctx context.Context, executionID string) error {
	session, err := m.repo.GetTaskSession(ctx, m.sessionID)
	if err != nil {
		return err
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.UpdatedAt = time.Now().UTC()
	if err := m.repo.UpdateTaskSession(ctx, session); err != nil {
		return err
	}
	return m.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "candidate-running",
		SessionID:        m.sessionID,
		TaskID:           m.taskID,
		AgentExecutionID: executionID,
		Status:           "ready",
	})
}

func (m *continuationAdmissionAgentManager) CleanupStaleExecutionBySessionID(
	ctx context.Context,
	sessionID string,
) error {
	return m.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
}

type continuationGenerationCASFailureRepository struct {
	*sqliterepo.Repository
	commitErr error
}

func (r *continuationGenerationCASFailureRepository) CommitHarnessSessionGeneration(
	context.Context,
	*models.HarnessSessionGeneration,
	int64,
) (bool, error) {
	return false, r.commitErr
}

func TestRecoverSession_ContextContinuationCommitsBeforePromptAdmission(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	taskID := "task-continuation-admission"
	sessionID := "session-continuation-admission"
	seedTaskAndSession(t, baseRepo, taskID, sessionID, models.TaskSessionStateFailed)

	now := time.Now().UTC()
	if err := baseRepo.CreateExecutor(ctx, &models.Executor{
		ID:        "executor-continuation-admission",
		Name:      "Worktree",
		Type:      models.ExecutorTypeWorktree,
		Status:    models.ExecutorStatusActive,
		Resumable: true,
	}); err != nil {
		t.Fatalf("create executor: %v", err)
	}
	if err := baseRepo.CreateRepository(ctx, &models.Repository{
		ID:            "repo-continuation-admission",
		WorkspaceID:   "ws1",
		Name:          "backend",
		SourceType:    "local",
		LocalPath:     t.TempDir(),
		DefaultBranch: "main",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := baseRepo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID:           "task-repo-continuation-admission",
		TaskID:       taskID,
		RepositoryID: "repo-continuation-admission",
		BaseBranch:   "main",
		Position:     0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session, err := baseRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile-continuation-admission"
	session.ExecutorID = "executor-continuation-admission"
	session.RepositoryID = "repo-continuation-admission"
	session.BaseBranch = "main"
	if err := baseRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	if err := baseRepo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID:                 "continuation-admission-block",
		SessionID:          sessionID,
		IncarnationID:      sessionID,
		ExpectedGeneration: 0,
		Reason:             "native_state_missing",
		State:              models.RecoveryBlockOpen,
	}); err != nil {
		t.Fatalf("persist recovery block: %v", err)
	}

	commitErr := errors.New("continuation generation CAS failed")
	repo := &continuationGenerationCASFailureRepository{
		Repository: baseRepo,
		commitErr:  commitErr,
	}
	manager := &continuationAdmissionAgentManager{
		mockAgentManager: &mockAgentManager{
			isAgentReadyFn: func(context.Context, string) bool {
				return true
			},
			getACPSessionIDForSessionFunc: func(string) (string, bool) {
				return "native-continuation-admission", true
			},
		},
		repo:      baseRepo,
		taskID:    taskID,
		sessionID: sessionID,
	}
	promptStarted := make(chan struct{})
	promptRelease := make(chan struct{})
	manager.promptAgentFunc = func(
		context.Context,
		string,
		string,
		[]v1.MessageAttachment,
		bool,
	) (*executor.PromptResult, error) {
		close(promptStarted)
		<-promptRelease
		return &executor.PromptResult{}, nil
	}
	svc := createTestServiceWithAgent(baseRepo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.repo = repo
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.RecoverSession(ctx, taskID, sessionID, sessionRecoveryActionContinueFromHistory)
	if !errors.Is(err, commitErr) {
		t.Fatalf("RecoverSession error = %v, want %v", err, commitErr)
	}
	select {
	case <-promptStarted:
		close(promptRelease)
		t.Fatal("continuation prompt was admitted before generation commit")
	default:
	}
	close(promptRelease)
	block, err := baseRepo.GetSessionRecoveryBlock(ctx, "continuation-admission-block")
	if err != nil {
		t.Fatalf("load recovery block: %v", err)
	}
	if block.State != models.RecoveryBlockOpen {
		t.Fatalf("recovery block state = %s, want open after failed commit", block.State)
	}

	running, err := baseRepo.GetExecutorRunningBySessionID(ctx, sessionID)
	if !errors.Is(err, models.ErrExecutorRunningNotFound) || running != nil {
		t.Fatalf("candidate execution after failed commit = %+v, err=%v; want cleaned up", running, err)
	}
}

type continuationCandidateCleanupAgentManager struct {
	*mockAgentManager
	cleanupCalls int
}

func (m *continuationCandidateCleanupAgentManager) CleanupStaleExecutionBySessionIDIfCurrent(
	_ context.Context,
	_, _ string,
	_ time.Time,
) error {
	m.cleanupCalls++
	return nil
}

func TestRollbackContinuationCandidateCleansOnlyCurrentCandidate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-continuation-rollback", "session-continuation-rollback", models.TaskSessionStateFailed)
	session, err := repo.GetTaskSession(ctx, "session-continuation-rollback")
	if err != nil {
		t.Fatalf("load seeded session: %v", err)
	}
	session.ErrorMessage = "native state missing"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("persist seeded session error: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "candidate-row",
		SessionID:        "session-continuation-rollback",
		TaskID:           "task-continuation-rollback",
		AgentExecutionID: "candidate-execution",
		Status:           "ready",
	}); err != nil {
		t.Fatalf("seed candidate execution: %v", err)
	}
	agentManager := &continuationCandidateCleanupAgentManager{mockAgentManager: &mockAgentManager{}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	checkpoint := &continuationCheckpoint{
		sessionID:            "session-continuation-rollback",
		candidateExecutionID: "candidate-execution",
		previousState:        models.TaskSessionStateFailed,
		previousErrorMessage: "native state missing",
	}
	if err := service.rollbackContinuationCandidate(ctx, checkpoint); err != nil {
		t.Fatalf("rollbackContinuationCandidate: %v", err)
	}
	if agentManager.cleanupCalls != 1 {
		t.Fatalf("candidate cleanup calls = %d, want 1", agentManager.cleanupCalls)
	}
	session, err = repo.GetTaskSession(ctx, checkpoint.sessionID)
	if err != nil {
		t.Fatalf("load rolled-back session: %v", err)
	}
	if session.State != checkpoint.previousState || session.ErrorMessage != checkpoint.previousErrorMessage {
		t.Fatalf("rolled-back session = %+v, want state=%q error=%q", session, checkpoint.previousState, checkpoint.previousErrorMessage)
	}
}

// --- launchRestoreWorkspace ---

func TestLaunchRestoreWorkspace_MissingSessionID(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	_, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID: "task1",
		Intent: IntentRestoreWorkspace,
	})
	if err == nil {
		t.Fatal("expected error when session_id is empty")
	}
}

func TestLaunchRestoreWorkspace_SessionNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	_, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID:    "task1",
		Intent:    IntentRestoreWorkspace,
		SessionID: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error when session does not exist")
	}
}

func TestLaunchRestoreWorkspace_WrongTask(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task-other", "session1", models.TaskSessionStateCompleted)

	_, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID:    "task-wrong",
		Intent:    IntentRestoreWorkspace,
		SessionID: "session1",
	})
	if err == nil {
		t.Fatal("expected error when session does not belong to task")
	}
}

func TestLaunchRestoreWorkspace_Success(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCompleted)

	resp, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID:    "task1",
		Intent:    IntentRestoreWorkspace,
		SessionID: "session1",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if resp.SessionID != "session1" {
		t.Errorf("expected session_id 'session1', got %q", resp.SessionID)
	}
	if resp.State != string(models.TaskSessionStateCompleted) {
		t.Errorf("expected state %q, got %q", models.TaskSessionStateCompleted, resp.State)
	}
}

// --- launchPrepare passthrough upgrade ---

func TestIsPassthroughProfile(t *testing.T) {
	repo := setupTestRepo(t)
	passthroughMgr := &mockAgentManager{isPassthrough: true}
	regularMgr := &mockAgentManager{isPassthrough: false}

	t.Run("passthrough profile detected", func(t *testing.T) {
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), passthroughMgr)
		if !svc.isPassthroughProfile(context.Background(), "profile1") {
			t.Error("expected isPassthroughProfile=true for passthrough profile")
		}
	})

	t.Run("non-passthrough profile not detected", func(t *testing.T) {
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), regularMgr)
		if svc.isPassthroughProfile(context.Background(), "profile1") {
			t.Error("expected isPassthroughProfile=false for non-passthrough profile")
		}
	})

	t.Run("empty profile id returns false", func(t *testing.T) {
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), passthroughMgr)
		if svc.isPassthroughProfile(context.Background(), "") {
			t.Error("expected isPassthroughProfile=false for empty profile id")
		}
	})

	t.Run("nil agent manager returns false", func(t *testing.T) {
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.agentManager = nil
		if svc.isPassthroughProfile(context.Background(), "profile1") {
			t.Error("expected isPassthroughProfile=false when agent manager is nil")
		}
	})

	t.Run("resolver error returns false", func(t *testing.T) {
		errorMgr := &mockAgentManager{resolveProfileErr: errors.New("lookup failed")}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), errorMgr)
		if svc.isPassthroughProfile(context.Background(), "profile1") {
			t.Error("expected isPassthroughProfile=false when resolver errors")
		}
	})
}

// TestShouldUpgradePassthroughPrepare pins the launchPrepare upgrade decision.
//
// Regression guard for the prompt-delivery break introduced by the passthrough
// upgrade (PR #744): the two-phase create flow does a cheap prompt-less prepare
// followed by an async IntentStartCreated that carries the prompt. If a
// passthrough prepare is eagerly upgraded to a full launch there, the PTY spawns
// with an empty prompt and the prompt-bearing start is rejected against the
// now-running session — so the agent sits at an empty prompt. DeferredStart must
// suppress the upgrade so that follow-up start launches the passthrough WITH the
// prompt, exactly as ACP already does.
func TestShouldUpgradePassthroughPrepare(t *testing.T) {
	repo := setupTestRepo(t)
	passthrough := &mockAgentManager{isPassthrough: true}
	regular := &mockAgentManager{isPassthrough: false}

	tests := []struct {
		name string
		mgr  *mockAgentManager
		req  LaunchSessionRequest
		want bool
	}{
		{
			name: "prepare-only passthrough upgrades (quick-chat terminal needs a PTY)",
			mgr:  passthrough,
			req:  LaunchSessionRequest{AgentProfileID: "profile1"},
			want: true,
		},
		{
			name: "deferred-start passthrough does NOT upgrade (prompt-bearing start follows)",
			mgr:  passthrough,
			req:  LaunchSessionRequest{AgentProfileID: "profile1", DeferredStart: true},
			want: false,
		},
		{
			name: "auto-start passthrough does NOT upgrade (avoids launchStart bounce)",
			mgr:  passthrough,
			req:  LaunchSessionRequest{AgentProfileID: "profile1", AutoStart: true},
			want: false,
		},
		{
			name: "non-passthrough never upgrades",
			mgr:  regular,
			req:  LaunchSessionRequest{AgentProfileID: "profile1"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), tt.mgr)
			if got := svc.shouldUpgradePassthroughPrepare(context.Background(), &tt.req); got != tt.want {
				t.Errorf("shouldUpgradePassthroughPrepare()=%v, want %v", got, tt.want)
			}
		})
	}
}

// TestLaunchPrepare_PassthroughDoesNotRecurse guards against an infinite
// launchStart ↔ launchPrepare bounce when a passthrough profile is combined
// with AutoStart=true and a step that blocks auto-start.
//
// To actually exercise the guard, the task must be persisted with a
// WorkflowStepID that maps to a step lacking `auto_start_agent` — otherwise
// `shouldBlockAutoStart` short-circuits to false and the test bypasses the
// downgrade path entirely. Wiring the full scheduler+executor stack is too
// heavy, so we run in a goroutine with a panic recover: stack-overflow
// recursion would never return, while the legitimate downstream nil-deref
// panic from the stub scheduler still completes within the deadline.
func TestLaunchPrepare_PassthroughDoesNotRecurse(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := &mockAgentManager{isPassthrough: true}
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-blocked"] = &wfmodels.WorkflowStep{
		ID:   "step-blocked",
		Name: "blocked",
		// no on_enter actions => auto_start_agent missing => blocked
	}
	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, mgr)

	// Persist a task with the blocking step so shouldBlockAutoStart returns
	// true, forcing launchStart to downgrade into launchPrepare. Without the
	// `!req.AutoStart` guard, that re-enters launchStart and recurses.
	seedTaskAndSessionWithStep(t, repo, "task1", "sess-pass", "step-blocked")

	done := make(chan struct{})
	go func() {
		defer func() {
			// Stub scheduler nil-deref is expected. Recursion would never reach
			// this point.
			_ = recover()
			close(done)
		}()
		_, _ = svc.LaunchSession(context.Background(), &LaunchSessionRequest{
			TaskID:         "task1",
			Intent:         IntentStart,
			AgentProfileID: "profile-pass",
			AutoStart:      true,
			WorkflowStepID: "step-blocked",
		})
	}()

	select {
	case <-done:
		// returned without recursing
	case <-time.After(2 * time.Second):
		t.Fatal("LaunchSession recursed indefinitely (timed out)")
	}
}

// seedTaskAndSessionWithStep is a variant of seedTaskAndSession that wires
// the task to a workflow step, required by shouldBlockAutoStart.
func seedTaskAndSessionWithStep(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, stepID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)

	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test Workflow", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)

	task := &models.Task{
		ID:             taskID,
		WorkflowID:     "wf1",
		WorkflowStepID: stepID,
		Title:          "Test Task",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	session := &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     models.TaskSessionStateCreated,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
}

func TestLaunchRestoreWorkspace_IncludesWorktreeInfo(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	// Add worktree to the session's environment
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env1", TaskID: "task1", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.TaskEnvironmentID = "env1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link session to environment: %v", err)
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID:                "wt1",
		TaskEnvironmentID: "env1",
		WorktreeID:        "wid1",
		RepositoryID:      "repo1",
		WorktreePath:      "/tmp/worktrees/session1",
		WorktreeBranch:    "feature/test",
	}); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	resp, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:    "task1",
		Intent:    IntentRestoreWorkspace,
		SessionID: "session1",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.WorktreePath == nil || *resp.WorktreePath != "/tmp/worktrees/session1" {
		t.Errorf("expected worktree_path '/tmp/worktrees/session1', got %v", resp.WorktreePath)
	}
	if resp.WorktreeBranch == nil || *resp.WorktreeBranch != "feature/test" {
		t.Errorf("expected worktree_branch 'feature/test', got %v", resp.WorktreeBranch)
	}
}

func TestIsBenignLaunchTeardownErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{
			name: "wrapped context.Canceled",
			err:  fmt.Errorf("launch failed: %w", context.Canceled),
			want: true,
		},
		{
			name: "wrapped ErrSessionTerminal",
			err:  fmt.Errorf("restore workspace: %w", lifecycle.ErrSessionTerminal),
			want: true,
		},
		{
			// resume path stringifies a persisted session error, destroying the
			// sentinel (task_operations.go uses %s): the string fallback catches it.
			name: "stringified context canceled (resume)",
			err:  fmt.Errorf("session failed: %s", "context canceled"),
			want: true,
		},
		{
			name: "stringified session is terminal",
			err:  errors.New("session failed: session is terminal"),
			want: true,
		},
		{
			name: "unknown task is not benign",
			err:  errors.New("task not found"),
			want: false,
		},
		{
			name: "validation error is not benign",
			err:  errors.New("task_id is required"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBenignLaunchTeardownErr(tt.err); got != tt.want {
				t.Fatalf("IsBenignLaunchTeardownErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
