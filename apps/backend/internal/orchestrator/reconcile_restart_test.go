package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// errWorkTransient is a non-not-found stop error used to assert that a transient
// stop failure never causes a confirmed-dead row to be pruned.
var errWorkTransient = errors.New("runtime stop transient failure")

// TestReconcileSessionsOnStartupMakesRowsTrue is the restart-reconciliation
// integration test for #1597 startup reconciliation (the backlog stops
// growing, rows are made true, resumability is never lost). After a restart,
// reconciliation verifies each executors_running
// row against reality using runtime-aware liveness and:
//   - prunes a terminal row whose process is dead and that holds no resume_token
//     (the stale-row backlog trends toward zero);
//   - repairs a resumable row whose process is dead in place — status=stopped,
//     local_pid cleared, resume_token preserved — so it never keeps claiming a
//     live process, yet stays resumable;
//   - leaves a row whose process is still alive with its live handle intact;
//   - never applies the local liveness check to an SSH (remote) row.
func TestReconcileSessionsOnStartupMakesRowsTrue(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskA", "sA", models.TaskSessionStateCompleted)       // terminal, no token, dead → prune
	seedTaskAndSession(t, repo, "taskB", "sB", models.TaskSessionStateWaitingForInput) // resumable + token, dead → repair
	seedTaskAndSession(t, repo, "taskC", "sC", models.TaskSessionStateRunning)         // alive → preserve live handle
	seedTaskAndSession(t, repo, "taskD", "sD", models.TaskSessionStateWaitingForInput) // SSH remote → Unknown, untouched

	upsert := func(er *models.ExecutorRunning) {
		er.CreatedAt, er.UpdatedAt = now, now
		if err := repo.UpsertExecutorRunning(ctx, er); err != nil {
			t.Fatalf("upsert %s: %v", er.SessionID, err)
		}
	}
	upsert(&models.ExecutorRunning{ID: "sA", SessionID: "sA", TaskID: "taskA", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning, LocalPID: 111})
	upsert(&models.ExecutorRunning{ID: "sB", SessionID: "sB", TaskID: "taskB", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning, ResumeToken: "tokB", Resumable: true, LocalPID: 222})
	upsert(&models.ExecutorRunning{ID: "sC", SessionID: "sC", TaskID: "taskC", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning, LocalPID: 333})
	upsert(&models.ExecutorRunning{ID: "sD", SessionID: "sD", TaskID: "taskD", Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning, ResumeToken: "tokD", PID: 444})

	agentMgr := &mockAgentManager{
		rowLivenessFn: func(r *models.ExecutorRunning) models.ProcessLiveness {
			switch r.SessionID {
			case "sA", "sB":
				return models.ProcessLivenessDead
			case "sC":
				return models.ProcessLivenessAlive
			default: // sD (SSH) — a local check must never judge a remote row
				return models.ProcessLivenessUnknown
			}
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	// sA: terminal + no resume_token + dead → pruned.
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sA"); err == nil {
		t.Error("sA: terminal dead row with no resume_token should be pruned")
	}

	// sB: resumable + token + dead → preserved AND repaired.
	b, err := repo.GetExecutorRunningBySessionID(ctx, "sB")
	if err != nil {
		t.Fatalf("sB should be preserved: %v", err)
	}
	if b.ResumeToken != "tokB" {
		t.Errorf("sB resume_token lost during repair: %q", b.ResumeToken)
	}
	if b.Status != models.ExecutorRunningStatusStopped || b.LocalPID != 0 {
		t.Errorf("sB should be repaired to stopped with cleared local_pid; got status=%q local_pid=%d", b.Status, b.LocalPID)
	}

	// sC: alive → live local handle preserved (not repaired away).
	c, err := repo.GetExecutorRunningBySessionID(ctx, "sC")
	if err != nil {
		t.Fatalf("sC should be preserved: %v", err)
	}
	if c.LocalPID != 333 {
		t.Errorf("sC live local handle must be preserved; got local_pid=%d", c.LocalPID)
	}

	// sD: SSH → local liveness Unknown; the local reconcile must not touch it.
	d, err := repo.GetExecutorRunningBySessionID(ctx, "sD")
	if err != nil {
		t.Fatalf("sD should be preserved: %v", err)
	}
	if d.PID != 444 || d.ResumeToken != "tokD" {
		t.Errorf("sD SSH row must be untouched by the local reconcile; got pid=%d token=%q", d.PID, d.ResumeToken)
	}
}

// TestReconcileSessionsOnStartup_IdleSessionDeadRowRepaired covers the office
// IDLE path: an office turn writes IDLE and tears down, so a crash/restart in
// that window leaves a row claiming status=running with a dead local_pid.
// Startup reconciliation must repair the row (stopped, local_pid cleared,
// resume_token preserved) WITHOUT flipping the session out of IDLE — the IDLE
// state is the office "between turns" shape and must be preserved (#1597).
func TestReconcileSessionsOnStartup_IdleSessionDeadRowRepaired(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskI", "sI", models.TaskSessionStateIdle)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sI", SessionID: "sI", TaskID: "taskI", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, ResumeToken: "tokI", Resumable: true,
		LocalPID: 4343, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sI: %v", err)
	}

	agentMgr := &mockAgentManager{
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.reconcileSessionsOnStartup(ctx)

	// Session state stays IDLE (not flipped to WAITING_FOR_INPUT).
	session, err := repo.GetTaskSession(ctx, "sI")
	if err != nil {
		t.Fatalf("GetTaskSession(sI): %v", err)
	}
	if session.State != models.TaskSessionStateIdle {
		t.Errorf("IDLE session state must be preserved; got %q", session.State)
	}
	// But the row is repaired so it no longer claims a live process.
	row, err := repo.GetExecutorRunningBySessionID(ctx, "sI")
	if err != nil {
		t.Fatalf("idle resumable row must be preserved: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusStopped || row.LocalPID != 0 {
		t.Errorf("dead idle row must be repaired to stopped with cleared local_pid; got status=%q local_pid=%d", row.Status, row.LocalPID)
	}
	if row.ResumeToken != "tokI" {
		t.Errorf("resume_token must survive the idle repair; got %q", row.ResumeToken)
	}
}

// TestReconcileSessionsOnStartup_MissingSessionStopsAgentAndDeletesRow covers
// the orphan-row branch of startup reconciliation: a row whose task_session is
// gone entirely (deleted task/worktree) routes to handleMissingSessionOnStartup,
// which stops the still-registered runtime handle (forced) and, once that
// succeeds, deletes the now-meaningless executors_running row — so orphan rows
// don't survive restarts and inflate the backlog (#1597).
func TestReconcileSessionsOnStartup_MissingSessionStopsAgentAndDeletesRow(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// No backing task_session row for "sO" — GetTaskSession returns not-found.
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "sO",
		SessionID:        "sO",
		TaskID:           "taskO",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusRunning,
		AgentExecutionID: "execO",
		LocalPID:         4242,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("upsert sO: %v", err)
	}

	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.reconcileSessionsOnStartup(ctx)

	if len(agentMgr.stopAgentWithReasonArgs) != 1 || agentMgr.stopAgentWithReasonArgs[0].ExecutionID != "execO" {
		t.Fatalf("orphan row must stop its runtime handle via StopAgentWithReason; got %+v", agentMgr.stopAgentWithReasonArgs)
	}
	if !agentMgr.stopAgentWithReasonArgs[0].Force {
		t.Errorf("missing-session stop must be forced")
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sO"); err == nil {
		t.Error("orphan row must be deleted once the runtime stop succeeds")
	}
}

// TestReconcileSessionsOnStartup_MissingSessionDeadRowPrunedOnNotFound is the
// regression test for the stale-orphan-row bug: a missing-session row whose
// runtime handle is already gone returns a not-found error from the stop, which
// the old code treated as "stop failed" and preserved forever. A confirmed-dead
// LOCAL row must instead treat the not-found stop as already-stopped and prune
// the row under the resume-safety invariant (no resume_token → delete).
func TestReconcileSessionsOnStartup_MissingSessionDeadRowPrunedOnNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sDead", SessionID: "sDead", TaskID: "taskDead", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execDead", LocalPID: 4242,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sDead: %v", err)
	}

	agentMgr := &mockAgentManager{
		// The runtime seam (lifecycleAdapter) normalizes the lifecycle not-found
		// sentinel to runtimeapi.ErrNotFound before it reaches the orchestrator.
		stopAgentWithReasonErr: fmt.Errorf("stop agent: %w", runtimeapi.ErrNotFound),
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sDead"); err == nil {
		t.Error("confirmed-dead local orphan row with a not-found stop must be pruned, not preserved")
	}
}

// TestReconcileSessionsOnStartup_MissingSessionDeadRowWithTokenRepaired covers
// the resume-safety half of the confirmed-dead not-found path: a dead local row
// that still carries a resume_token is repaired in place (never deleted).
func TestReconcileSessionsOnStartup_MissingSessionDeadRowWithTokenRepaired(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sTok", SessionID: "sTok", TaskID: "taskTok", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execTok",
		ResumeToken: "tokTok", Resumable: true, LocalPID: 5252,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sTok: %v", err)
	}

	agentMgr := &mockAgentManager{
		stopAgentWithReasonErr: fmt.Errorf("stop agent: %w", runtimeapi.ErrNotFound),
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	row, err := repo.GetExecutorRunningBySessionID(ctx, "sTok")
	if err != nil {
		t.Fatalf("confirmed-dead row holding a resume_token must be preserved: %v", err)
	}
	if row.ResumeToken != "tokTok" {
		t.Errorf("resume_token must survive repair; got %q", row.ResumeToken)
	}
	if row.Status != models.ExecutorRunningStatusStopped || row.LocalPID != 0 {
		t.Errorf("dead row must be repaired to stopped with cleared local_pid; got status=%q local_pid=%d", row.Status, row.LocalPID)
	}
}

// TestReconcileSessionsOnStartup_MissingSessionUnknownRowPreservedOnNotFound
// guards the anti-blanket-ignore rule: a not-found stop for a row whose local
// liveness is Unknown (remote/containerized/no local handle) must NOT prune the
// row — only a confirmed-dead LOCAL row is reclassified.
func TestReconcileSessionsOnStartup_MissingSessionUnknownRowPreservedOnNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sRemote", SessionID: "sRemote", TaskID: "taskRemote", Runtime: agentruntime.RuntimeSSH,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execRemote",
		ResumeToken: "tokRemote", PID: 4444, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sRemote: %v", err)
	}

	agentMgr := &mockAgentManager{
		stopAgentWithReasonErr: fmt.Errorf("stop agent: %w", runtimeapi.ErrNotFound),
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessUnknown
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	row, err := repo.GetExecutorRunningBySessionID(ctx, "sRemote")
	if err != nil {
		t.Fatalf("unknown/remote row must be preserved on a host-local not-found: %v", err)
	}
	if row.ResumeToken != "tokRemote" || row.PID != 4444 {
		t.Errorf("unknown/remote row must be left intact; got token=%q pid=%d", row.ResumeToken, row.PID)
	}
}

// TestReconcileSessionsOnStartup_MissingSessionNonNotFoundPreserved guards the
// retryable-failure rule: a stop that fails with a non-not-found error must
// preserve the row even when the row is confirmed dead — a transient stop
// failure is never mistaken for an absent runtime.
func TestReconcileSessionsOnStartup_MissingSessionNonNotFoundPreserved(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sErr", SessionID: "sErr", TaskID: "taskErr", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execErr", LocalPID: 6262,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sErr: %v", err)
	}

	agentMgr := &mockAgentManager{
		stopAgentWithReasonErr: errWorkTransient, // not a not-found sentinel
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sErr"); err != nil {
		t.Errorf("a non-not-found stop failure must preserve the row for retry: %v", err)
	}
}

// TestReconcileSessionsOnStartup_CreatedSessionRowPrunedUnlessResumable covers
// the never-started-session cleanup site, which routes through the resume-safety
// invariant rather than deleting unconditionally: a Created session's row with
// no resume_token is pruned (nothing to lose), while the rare Created row that
// already carries a resume_token is repaired in place — the token is the only
// handle to the agent-side conversation and must survive
// (#1597 resume-safety invariant).
func TestReconcileSessionsOnStartup_CreatedSessionRowPrunedUnlessResumable(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskE", "sE", models.TaskSessionStateCreated) // no token → prune
	seedTaskAndSession(t, repo, "taskF", "sF", models.TaskSessionStateCreated) // token → repair

	upsert := func(er *models.ExecutorRunning) {
		er.CreatedAt, er.UpdatedAt = now, now
		if err := repo.UpsertExecutorRunning(ctx, er); err != nil {
			t.Fatalf("upsert %s: %v", er.SessionID, err)
		}
	}
	upsert(&models.ExecutorRunning{ID: "sE", SessionID: "sE", TaskID: "taskE", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting, LocalPID: 555})
	upsert(&models.ExecutorRunning{ID: "sF", SessionID: "sF", TaskID: "taskF", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting, ResumeToken: "tokF", Resumable: true, LocalPID: 666})

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sE"); err == nil {
		t.Error("sE: never-started row with no resume_token should be pruned")
	}

	f, err := repo.GetExecutorRunningBySessionID(ctx, "sF")
	if err != nil {
		t.Fatalf("sF: Created row holding a resume_token must be preserved: %v", err)
	}
	if f.ResumeToken != "tokF" {
		t.Errorf("sF resume_token lost: %q", f.ResumeToken)
	}
	if f.Status != models.ExecutorRunningStatusStopped || f.LocalPID != 0 {
		t.Errorf("sF should be repaired to stopped with cleared local_pid; got status=%q local_pid=%d", f.Status, f.LocalPID)
	}
}

// TestReconcileSessionsOnStartup_TerminalDeadRowPrunedOnNotFound is the
// regression test for the "failed to stop terminal session runtime; preserving
// executor record" warning: a COMPLETED/CANCELLED session whose confirmed-dead
// LOCAL runtime returns a not-found stop must be treated as already-stopped and
// its tokenless row pruned — not preserved forever. Mirrors the missing-session
// classification so terminal reconciliation stops leaking stale rows.
func TestReconcileSessionsOnStartup_TerminalDeadRowPrunedOnNotFound(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			repo := setupTestRepo(t)
			ctx := context.Background()
			now := time.Now().UTC()

			seedTaskAndSession(t, repo, "taskT", "sT", state)
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "sT", SessionID: "sT", TaskID: "taskT", Runtime: agentruntime.RuntimeStandalone,
				Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execT", LocalPID: 7272,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("upsert sT: %v", err)
			}

			agentMgr := &mockAgentManager{
				stopAgentWithReasonErr: fmt.Errorf("stop agent: %w", runtimeapi.ErrNotFound),
				rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
					return models.ProcessLivenessDead
				},
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			svc.reconcileSessionsOnStartup(ctx)

			if _, err := repo.GetExecutorRunningBySessionID(ctx, "sT"); err == nil {
				t.Error("confirmed-dead terminal row with a not-found stop must be pruned, not preserved")
			}
		})
	}
}

// TestReconcileSessionsOnStartup_TerminalDeadRowWithTokenRepaired covers the
// resume-safety half of the terminal not-found path: a terminal row still
// carrying a resume_token is repaired in place, never deleted.
func TestReconcileSessionsOnStartup_TerminalDeadRowWithTokenRepaired(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskTT", "sTT", models.TaskSessionStateCompleted)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sTT", SessionID: "sTT", TaskID: "taskTT", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execTT",
		ResumeToken: "tokTT", Resumable: true, LocalPID: 8282,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sTT: %v", err)
	}

	agentMgr := &mockAgentManager{
		stopAgentWithReasonErr: fmt.Errorf("stop agent: %w", runtimeapi.ErrNotFound),
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	row, err := repo.GetExecutorRunningBySessionID(ctx, "sTT")
	if err != nil {
		t.Fatalf("terminal row holding a resume_token must be preserved: %v", err)
	}
	if row.ResumeToken != "tokTT" {
		t.Errorf("resume_token must survive repair; got %q", row.ResumeToken)
	}
	if row.Status != models.ExecutorRunningStatusStopped || row.LocalPID != 0 {
		t.Errorf("dead terminal row must be repaired to stopped with cleared local_pid; got status=%q local_pid=%d", row.Status, row.LocalPID)
	}
}

// TestReconcileSessionsOnStartup_TerminalUnknownRowPreservedOnNotFound guards the
// anti-blanket-ignore rule for the terminal path: a not-found stop for a row
// whose local liveness is Unknown (remote/no local handle) must NOT prune the
// row — only a confirmed-dead LOCAL row is reclassified.
func TestReconcileSessionsOnStartup_TerminalUnknownRowPreservedOnNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskTU", "sTU", models.TaskSessionStateCancelled)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sTU", SessionID: "sTU", TaskID: "taskTU", Runtime: agentruntime.RuntimeSSH,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execTU",
		PID: 9191, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sTU: %v", err)
	}

	agentMgr := &mockAgentManager{
		stopAgentWithReasonErr: fmt.Errorf("stop agent: %w", runtimeapi.ErrNotFound),
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessUnknown
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sTU"); err != nil {
		t.Fatalf("unknown/remote terminal row must be preserved on a host-local not-found: %v", err)
	}
}

// TestReconcileSessionsOnStartup_FailedNonResumableDeadRowPrunedOnNotFound
// extends the classification to the non-resumable FAILED startup path: a failed
// session with no resume handle whose dead-local runtime returns not-found must
// have its row pruned rather than preserved with the "failed to stop failed
// session runtime; preserving executor record" warning.
func TestReconcileSessionsOnStartup_FailedNonResumableDeadRowPrunedOnNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskFail", "sFail", models.TaskSessionStateFailed)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sFail", SessionID: "sFail", TaskID: "taskFail", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execFail", LocalPID: 3131,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sFail: %v", err)
	}

	agentMgr := &mockAgentManager{
		stopAgentWithReasonErr: fmt.Errorf("stop agent: %w", runtimeapi.ErrNotFound),
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sFail"); err == nil {
		t.Error("confirmed-dead non-resumable failed row with a not-found stop must be pruned, not preserved")
	}
}

// TestReconcileSessionsOnStartup_TerminalNonNotFoundPreserved guards the
// retryable-failure rule for the terminal path: a stop that fails with a
// non-not-found error must preserve the row even when the row is confirmed dead.
func TestReconcileSessionsOnStartup_TerminalNonNotFoundPreserved(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskTE", "sTE", models.TaskSessionStateCompleted)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sTE", SessionID: "sTE", TaskID: "taskTE", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execTE", LocalPID: 4141,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sTE: %v", err)
	}

	agentMgr := &mockAgentManager{
		stopAgentWithReasonErr: errWorkTransient, // not a not-found sentinel
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sTE"); err != nil {
		t.Errorf("a non-not-found stop failure must preserve the terminal row for retry: %v", err)
	}
}
