package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// inactiveTurnService reports no active turn for any session, satisfying the
// idle-reclaim precondition that no in-flight turn is bound to the session.
// Only GetActiveTurn is exercised by the reclaim primitive; other methods
// panic so a test that grows new behavior fails loudly.
type inactiveTurnService struct{}

func (*inactiveTurnService) GetActiveTurn(_ context.Context, _ string) (*models.Turn, error) {
	return nil, nil
}
func (*inactiveTurnService) StartTurn(context.Context, string) (*models.Turn, error) {
	panic("inactiveTurnService: StartTurn should not be called by reclaim tests")
}
func (*inactiveTurnService) ReserveTurn(context.Context, string, *models.PromptDispatchRecovery) (*models.Turn, error) {
	panic("inactiveTurnService: ReserveTurn should not be called by reclaim tests")
}
func (*inactiveTurnService) MarkReservedTurnDispatchAttempted(context.Context, *models.Turn) error {
	panic("inactiveTurnService: MarkReservedTurnDispatchAttempted should not be called by reclaim tests")
}
func (*inactiveTurnService) PublishReservedTurn(context.Context, *models.Turn) error {
	panic("inactiveTurnService: PublishReservedTurn should not be called by reclaim tests")
}
func (*inactiveTurnService) RollbackReservedTurn(context.Context, string, string) (bool, error) {
	panic("inactiveTurnService: RollbackReservedTurn should not be called by reclaim tests")
}
func (*inactiveTurnService) ReconcileUnpublishedPromptTurns(context.Context) (int, error) {
	return 0, nil
}
func (*inactiveTurnService) CompleteTurn(context.Context, string) error {
	panic("inactiveTurnService: CompleteTurn should not be called by reclaim tests")
}
func (*inactiveTurnService) GetTurn(context.Context, string) (*models.Turn, error) {
	panic("inactiveTurnService: GetTurn should not be called by reclaim tests")
}
func (*inactiveTurnService) UpdateTurn(context.Context, *models.Turn) error {
	panic("inactiveTurnService: UpdateTurn should not be called by reclaim tests")
}
func (*inactiveTurnService) PatchTurnMetadata(context.Context, string, string, map[string]interface{}) error {
	panic("inactiveTurnService: PatchTurnMetadata should not be called by reclaim tests")
}
func (*inactiveTurnService) AbandonOpenTurns(context.Context, string) error {
	return nil
}

// TestClassifyIdleReclaimDisposition is the single decision-matrix test for
// the idle-reclaim predicate. Each case names the (state, runtime-live,
// active-turn) tuple the primitive sees and the disposition it must return;
// fail-closed means every uncertain input is a Skipped* disposition.
func TestClassifyIdleReclaimDisposition(t *testing.T) {
	tests := []struct {
		name          string
		state         models.TaskSessionState
		agentRunning  bool
		hasActiveTurn bool
		want          idleReclaimDisposition
	}{
		{
			name:          "waiting_for_input, no live runtime, no active turn reclaims",
			state:         models.TaskSessionStateWaitingForInput,
			agentRunning:  false,
			hasActiveTurn: false,
			want:          idleReclaimDispositionReclaimed,
		},
		{
			name:          "idle office state, no live runtime, no active turn reclaims",
			state:         models.TaskSessionStateIdle,
			agentRunning:  false,
			hasActiveTurn: false,
			want:          idleReclaimDispositionReclaimed,
		},
		{
			name:          "running session is never reclaimed",
			state:         models.TaskSessionStateRunning,
			agentRunning:  false,
			hasActiveTurn: false,
			want:          idleReclaimDispositionSkippedState,
		},
		{
			name:          "starting session is never reclaimed",
			state:         models.TaskSessionStateStarting,
			agentRunning:  false,
			hasActiveTurn: false,
			want:          idleReclaimDispositionSkippedState,
		},
		{
			name:          "completed session with no live runtime reclaims",
			state:         models.TaskSessionStateCompleted,
			agentRunning:  false,
			hasActiveTurn: false,
			want:          idleReclaimDispositionReclaimed,
		},
		{
			name:          "failed session is never reclaimed",
			state:         models.TaskSessionStateFailed,
			agentRunning:  false,
			hasActiveTurn: false,
			want:          idleReclaimDispositionSkippedState,
		},
		{
			name:          "live runtime blocks reclaim",
			state:         models.TaskSessionStateWaitingForInput,
			agentRunning:  true,
			hasActiveTurn: false,
			want:          idleReclaimDispositionSkippedLive,
		},
		{
			name:          "active turn blocks reclaim",
			state:         models.TaskSessionStateWaitingForInput,
			agentRunning:  false,
			hasActiveTurn: true,
			want:          idleReclaimDispositionSkippedTurn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyIdleReclaim(tt.state, tt.agentRunning, tt.hasActiveTurn)
			if got != tt.want {
				t.Fatalf("disposition = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReclaimIdleSessionReleasesRuntimeAndPreservesRow asserts the happy
// path: an idle-yet-resumable session with no live runtime and no active
// turn gets its provider runtime released and its executors_running row
// repaired (status=stopped, local_pid=0) while the resume_token and
// worktree_path are preserved verbatim.
func TestReclaimIdleSessionReleasesRuntimeAndPreservesRow(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedTaskAndSession(t, repo, "taskIdle", "sessionIdle", models.TaskSessionStateWaitingForInput)
	const token = "resume-token-keep"
	const worktree = "/worktrees/keep"
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "sessionIdle",
		SessionID:        "sessionIdle",
		TaskID:           "taskIdle",
		AgentExecutionID: "exec-idle",
		ResumeToken:      token,
		WorktreePath:     worktree,
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusRunning,
		LocalPID:         4242,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("upsert idle row: %v", err)
	}

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		isAgentRunning: false,
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	})
	svc.turnService = &inactiveTurnService{}

	if err := svc.reclaimIdleSession(ctx, "sessionIdle"); err != nil {
		t.Fatalf("reclaimIdleSession: %v", err)
	}

	row, err := repo.GetExecutorRunningBySessionID(ctx, "sessionIdle")
	if err != nil {
		t.Fatalf("row missing after reclaim: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("status = %q, want %q", row.Status, models.ExecutorRunningStatusStopped)
	}
	if row.LocalPID != 0 {
		t.Fatalf("LocalPID = %d, want 0", row.LocalPID)
	}
	if row.ResumeToken != token {
		t.Fatalf("ResumeToken lost during reclaim: got %q, want %q", row.ResumeToken, token)
	}
	if row.WorktreePath != worktree {
		t.Fatalf("WorktreePath lost during reclaim: got %q, want %q", row.WorktreePath, worktree)
	}
}

// TestReclaimIdleSessionRefusesLiveRuntime proves the fail-closed guard:
// a session whose IsAgentRunningForSession returns true is never touched
// even when the session is in an idle state. The row keeps its running
// status and LocalPID; nothing is killed.
func TestReclaimIdleSessionRefusesLiveRuntime(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedTaskAndSession(t, repo, "taskLive", "sessionLive", models.TaskSessionStateWaitingForInput)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sessionLive", SessionID: "sessionLive", TaskID: "taskLive",
		AgentExecutionID: "exec-live", ResumeToken: "rt-keep",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		LocalPID: 7777, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert live row: %v", err)
	}

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		isAgentRunning: true, // critical: the agent is still alive
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessAlive
		},
	})
	svc.turnService = &inactiveTurnService{}

	if err := svc.reclaimIdleSession(ctx, "sessionLive"); err != nil {
		t.Fatalf("reclaimIdleSession: %v", err)
	}

	row, err := repo.GetExecutorRunningBySessionID(ctx, "sessionLive")
	if err != nil {
		t.Fatalf("row missing after reclaim: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("live session must not be stopped; status = %q", row.Status)
	}
	if row.LocalPID != 7777 {
		t.Fatalf("live session must not lose its PID; LocalPID = %d", row.LocalPID)
	}
}

type failingAgentLivenessProbe struct {
	*mockAgentManager
}

func (m *failingAgentLivenessProbe) ProbeAgentRunningForSession(context.Context, string) (bool, error) {
	return false, errors.New("agent status unavailable")
}

func TestReclaimIdleSessionFailsClosedOnLivenessProbeError(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedTaskAndSession(t, repo, "task-probe", "session-probe", models.TaskSessionStateWaitingForInput)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-probe", SessionID: "session-probe", TaskID: "task-probe",
		AgentExecutionID: "exec-probe", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, LocalPID: 99,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert probe row: %v", err)
	}

	base := &mockAgentManager{isAgentRunning: false}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &failingAgentLivenessProbe{mockAgentManager: base})
	svc.turnService = &inactiveTurnService{}

	if err := svc.reclaimIdleSession(ctx, "session-probe"); err != nil {
		t.Fatalf("reclaimIdleSession: %v", err)
	}
	row, err := repo.GetExecutorRunningBySessionID(ctx, "session-probe")
	if err != nil {
		t.Fatalf("row missing after uncertain probe: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusRunning || row.LocalPID != 99 {
		t.Fatalf("uncertain liveness must leave row untouched, got status=%q pid=%d", row.Status, row.LocalPID)
	}
}

// TestReclaimIdleSessionRefusesWrongState proves non-idle non-terminal
// states are never reclaimed: a RUNNING or STARTING session that happens
// to have no live runtime must wait for explicit completion, not get
// reaped by idle reclaim. Failed and Cancelled have dedicated cancellation
// cleanup paths (handleTerminalSessionOnStartup, the cancel pipelines)
// and are deliberately excluded from the reclaim predicate.
func TestReclaimIdleSessionRefusesWrongState(t *testing.T) {
	tests := []struct {
		name  string
		state models.TaskSessionState
	}{
		{name: "running", state: models.TaskSessionStateRunning},
		{name: "starting", state: models.TaskSessionStateStarting},
		{name: "failed", state: models.TaskSessionStateFailed},
		{name: "cancelled", state: models.TaskSessionStateCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			ctx := context.Background()
			now := time.Now().UTC()
			sessionID := "s-" + tt.name
			seedTaskAndSession(t, repo, "task-"+tt.name, sessionID, tt.state)
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: "task-" + tt.name,
				AgentExecutionID: "exec-" + tt.name,
				Runtime:          agentruntime.RuntimeStandalone,
				Status:           models.ExecutorRunningStatusRunning,
				LocalPID:         1234,
				CreatedAt:        now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("upsert: %v", err)
			}

			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
				isAgentRunning: false,
			})
			svc.turnService = &inactiveTurnService{}

			if err := svc.reclaimIdleSession(ctx, sessionID); err != nil &&
				!errors.Is(err, context.Canceled) {
				t.Fatalf("reclaimIdleSession: %v", err)
			}

			row, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
			if err != nil {
				t.Fatalf("row missing after reclaim: %v", err)
			}
			if row.Status != models.ExecutorRunningStatusRunning {
				t.Fatalf("%s session must not be reclaimed; status = %q", tt.state, row.Status)
			}
		})
	}
}

// TestReclaimIdleSessionMissingRowIsNoOp ensures the primitive is
// idempotent and safe to call when the executor row has already been
// pruned (e.g. by startup reconciliation).
func TestReclaimIdleSessionMissingRowIsNoOp(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "taskGone", "sessionGone", models.TaskSessionStateWaitingForInput)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		isAgentRunning: false,
	})
	svc.turnService = &inactiveTurnService{}

	if err := svc.reclaimIdleSession(ctx, "sessionGone"); err != nil {
		t.Fatalf("reclaimIdleSession with missing row: %v", err)
	}
}

// TestReclaimIdleSessionEmptySessionID is the trivial early return: an
// empty session ID never touches the database or the runtime.
func TestReclaimIdleSessionEmptySessionID(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	if err := svc.reclaimIdleSession(context.Background(), ""); err != nil {
		t.Fatalf("reclaimIdleSession with empty id: %v", err)
	}
}
