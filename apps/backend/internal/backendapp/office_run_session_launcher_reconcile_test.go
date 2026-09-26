package backendapp

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officecosts "github.com/kandev/kandev/internal/office/costs"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeReconcileBackend is a runtimeapi.Backend + runtimeapi.RunOwnerRecovery
// test double standing in for the lifecycle Manager. Launch records a live
// execution carrying the owner officeRunSessionLauncher.AdmitExecution
// admitted, so StartRunSession's real reserve/bind/admit/start sequence runs
// unmodified. evict then simulates a backend restart losing that execution
// from the recovered runtime inventory — the exact condition
// ReconcileRunSessions' "not found after restart" branch exists to detect.
type fakeReconcileBackend struct {
	mu            sync.Mutex
	executions    map[string]*lifecycle.AgentExecution
	nextID        int
	stoppedOwners []runtimeapi.ExecutionOwner
}

func newFakeReconcileBackend() *fakeReconcileBackend {
	return &fakeReconcileBackend{executions: map[string]*lifecycle.AgentExecution{}}
}

func (f *fakeReconcileBackend) Launch(_ context.Context, req *lifecycle.LaunchRequest) (*lifecycle.AgentExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("fake-exec-%d", f.nextID)
	exec := &lifecycle.AgentExecution{
		ID:             id,
		Owner:          req.Owner,
		OwnerAdmission: req.OwnerAdmission,
		AgentProfileID: req.AgentProfileID,
		WorkspaceID:    req.WorkspaceID,
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now().UTC(),
		ACPSessionID:   "acp-" + id,
	}
	f.executions[id] = exec
	return exec, nil
}

func (f *fakeReconcileBackend) StartAgentProcess(_ context.Context, _ string) error { return nil }

func (f *fakeReconcileBackend) PromptAgent(
	_ context.Context, _ string, _ string, _ []v1.MessageAttachment, _ bool,
) (*lifecycle.PromptResult, error) {
	return &lifecycle.PromptResult{}, nil
}

func (f *fakeReconcileBackend) StopAgentWithReason(_ context.Context, _ string, _ string, _ bool) error {
	return nil
}

func (f *fakeReconcileBackend) GetExecution(executionID string) (*lifecycle.AgentExecution, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	exec, ok := f.executions[executionID]
	return exec, ok
}

func (f *fakeReconcileBackend) SetMcpMode(_ context.Context, _ string, _ string) error { return nil }

// evict removes an execution, simulating a backend restart that lost the
// runtime's record of it.
func (f *fakeReconcileBackend) evict(executionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.executions, executionID)
}

func (f *fakeReconcileBackend) StopRunOwnerForRecovery(_ context.Context, owner runtimeapi.ExecutionOwner) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stoppedOwners = append(f.stoppedOwners, owner)
	return nil
}

func (f *fakeReconcileBackend) stopCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stoppedOwners)
}

var _ runtimeapi.Backend = (*fakeReconcileBackend)(nil)
var _ runtimeapi.RunOwnerRecovery = (*fakeReconcileBackend)(nil)

// reconcileHarness wires a real office/service.Service, a real
// officesqlite.Repository, and a real officeRunSessionLauncher over a fake
// runtime backend, so a test can drive officeRunSessionLauncher.StartRunSession
// and ReconcileRunSessions directly instead of reproducing their effects.
type reconcileHarness struct {
	svc      *officeservice.Service
	repo     *officesqlite.Repository
	eb       bus.EventBus
	launcher *officeRunSessionLauncher
	backend  *fakeReconcileBackend
}

func newReconcileHarness(t *testing.T) *reconcileHarness {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "reconcile.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	// Office agent CRUD reads/writes the merged agent_profiles table (ADR
	// 0005 Wave C), which only the settings store schema creates.
	if _, _, err := settingsstore.Provide(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("office repository: %v", err)
	}

	// agent_profiles.agent_id is a NOT NULL FK to agents (CLI tool
	// registrations); db.OpenSQLite enforces foreign keys, so seed one row
	// for CreateAgentInstance's DefaultAgentID fallback to resolve to.
	if _, err := sqlxDB.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"cli-agent-reconcile", "CLI Agent",
	); err != nil {
		t.Fatalf("seed agents row: %v", err)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)

	svc := officeservice.NewService(officeservice.ServiceOptions{
		Repo: officeRepo, Logger: log, EventBus: eventBus,
	})
	svc.SetSyncHandlers(true)
	if err := svc.RegisterEventSubscribers(eventBus); err != nil {
		t.Fatalf("register event subscribers: %v", err)
	}
	activity := shared.NewActivityLogger(officeRepo, log)
	svc.SetBudgetChecker(officecosts.NewCostService(officeRepo, log, activity, svc, svc))

	backend := newFakeReconcileBackend()
	launcher := newOfficeRunSessionLauncher(officeRepo, backend, log)
	svc.SetRunSessionLauncher(launcher)

	return &reconcileHarness{svc: svc, repo: officeRepo, eb: eventBus, launcher: launcher, backend: backend}
}

// TestContinuationSummary_InterruptedAttemptRecoveredAtStartup_PreservesPriorSummary
// pins the third failure mode: an attempt interrupted by a backend restart.
// Unlike a hand-simulated version, this drives the real
// officeRunSessionLauncher.ReconcileRunSessions against a real
// office_run_sessions row reserved, bound, and started by the real
// StartRunSession path, with only the runtime backend faked — proving the
// actual startup selection, liveness, and requeue branches, not a
// reproduction of their presumed effects.
func TestContinuationSummary_InterruptedAttemptRecoveredAtStartup_PreservesPriorSummary(t *testing.T) {
	ctx := context.Background()
	h := newReconcileHarness(t)

	agent := &officemodels.AgentInstance{
		ID: "taskless-interrupt-agent", WorkspaceID: "ws-1", Name: "taskless-interrupt-agent",
		Role: officemodels.AgentRoleCEO, Status: officemodels.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := h.svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// First attempt: launch and complete normally, producing the baseline
	// continuation summary that recovery must not disturb.
	if _, err := h.svc.QueueRun(ctx, agent.ID, officeservice.RunReasonRoutineTrigger, `{}`, "interrupt-first"); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	officeservice.RunSchedulerTick(h.svc, ctx)
	runs, err := h.svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list first run: runs=%#v err=%v", runs, err)
	}
	first := runs[0]
	firstSession, err := h.repo.GetRunSession(ctx, first.SessionID)
	if err != nil || firstSession == nil {
		t.Fatalf("get first run session %q: %v", first.SessionID, err)
	}
	if err := h.eb.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: firstSession.ExecutionID, AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: first.ID, RunSessionID: first.SessionID, RunAttempt: firstSession.Attempt, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish first completion: %v", err)
	}
	before, err := h.svc.GetContinuationSummaryForTest(ctx, agent.ID, "agent:"+agent.ID)
	if err != nil || before == nil || before.Content == "" {
		t.Fatalf("writer did not produce a baseline summary: %v (row=%v)", err, before)
	}

	// Second attempt: launch normally through the real StartRunSession path,
	// then never complete it — this is the interrupted attempt, still
	// carrying a live (per the fake backend) execution.
	if _, err := h.svc.QueueRun(ctx, agent.ID, officeservice.RunReasonRoutineTrigger, `{}`, "interrupt-second"); err != nil {
		t.Fatalf("queue second: %v", err)
	}
	officeservice.RunSchedulerTick(h.svc, ctx)
	runs, err = h.svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 2 {
		t.Fatalf("list second run: runs=%#v err=%v", runs, err)
	}
	var second *officemodels.Run
	for _, r := range runs {
		if r.ID != first.ID {
			second = r
		}
	}
	if second == nil || second.SessionID == "" {
		t.Fatalf("expected a second run with a bound session, got %#v", runs)
	}
	secondSession, err := h.repo.GetRunSession(ctx, second.SessionID)
	if err != nil || secondSession == nil || secondSession.ExecutionID == "" {
		t.Fatalf("get second run session %q: %v (session=%v)", second.SessionID, err, secondSession)
	}

	// Simulate a backend restart: the runtime's recovered inventory no
	// longer contains this execution.
	h.backend.evict(secondSession.ExecutionID)

	if err := h.launcher.ReconcileRunSessions(ctx); err != nil {
		t.Fatalf("reconcile run sessions: %v", err)
	}

	if got := h.backend.stopCallCount(); got != 1 {
		t.Fatalf("StopRunOwnerForRecovery calls = %d, want 1", got)
	}
	recovered, err := h.repo.GetRunSession(ctx, secondSession.ID)
	if err != nil || recovered == nil {
		t.Fatalf("get recovered session %q: %v", secondSession.ID, err)
	}
	if recovered.State != officemodels.RunSessionStateInterrupted || recovered.ErrorMessage == "" {
		t.Fatalf("recovered session = %#v, want state=interrupted with an error message", recovered)
	}
	runsAfterReconcile, err := h.svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil {
		t.Fatalf("list runs after reconcile: %v", err)
	}
	var secondAfterReconcile *officemodels.Run
	for _, r := range runsAfterReconcile {
		if r.ID == second.ID {
			secondAfterReconcile = r
		}
	}
	if secondAfterReconcile == nil || secondAfterReconcile.Status != officeservice.RunStatusQueued {
		t.Fatalf("second run after reconcile = %#v, want status %q", secondAfterReconcile, officeservice.RunStatusQueued)
	}

	// The recovery calls themselves must not have touched the summary.
	afterRecovery, err := h.svc.GetContinuationSummaryForTest(ctx, agent.ID, "agent:"+agent.ID)
	if err != nil || afterRecovery == nil {
		t.Fatalf("read summary after interrupted-attempt recovery: %v (row=%v)", err, afterRecovery)
	}
	if afterRecovery.Content != before.Content || afterRecovery.UpdatedByRunID != before.UpdatedByRunID {
		t.Fatalf("interrupted-attempt recovery clobbered continuation summary: before=%#v after=%#v", before, afterRecovery)
	}

	// The requeued run must still be able to relaunch and complete
	// normally, attributing a fresh summary write to itself without
	// disturbing the untouched baseline row's identity mid-flight.
	officeservice.RunSchedulerTick(h.svc, ctx)
	relaunched, err := h.repo.GetRun(ctx, second.ID)
	if err != nil || relaunched == nil || relaunched.SessionID == "" || relaunched.SessionID == second.SessionID {
		t.Fatalf("relaunched run = %#v, err=%v, want a new bound session distinct from %q", relaunched, err, second.SessionID)
	}
	relaunchedSession, err := h.repo.GetRunSession(ctx, relaunched.SessionID)
	if err != nil || relaunchedSession == nil {
		t.Fatalf("get relaunched session %q: %v", relaunched.SessionID, err)
	}
	if err := h.eb.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: relaunchedSession.ExecutionID, AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: second.ID, RunSessionID: relaunched.SessionID, RunAttempt: relaunchedSession.Attempt, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish relaunch completion: %v", err)
	}
	final, err := h.svc.GetContinuationSummaryForTest(ctx, agent.ID, "agent:"+agent.ID)
	if err != nil || final == nil {
		t.Fatalf("read summary after relaunch completion: %v (row=%v)", err, final)
	}
	if final.UpdatedByRunID != second.ID {
		t.Fatalf("summary after relaunch completion attributed to %q, want the recovered run %q",
			final.UpdatedByRunID, second.ID)
	}
}
