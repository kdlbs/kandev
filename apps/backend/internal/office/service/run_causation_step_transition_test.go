package service

// Unit tests for TaskBoundaryCarrierForStepTransition
// (AC-OFFICE-RUN-CAUSATION-001.25): a run queued because of a step-entry
// wake resolves its causation carrier from the task_step_transitions ledger
// row that caused the wake, not from a live claimed-run lookup scoped to a
// causing agent (there is no session on a step-entry ActionInput to scope
// by). Internal (package service) so TaskBoundaryCarrierForStepTransition
// can be called directly against a real sqlite-backed repo, reusing
// run_causation_from_task_test.go's harness plus a hand-rolled
// task_step_transitions table (that harness's minimal task schema doesn't
// include it — it's owned by internal/task/repository/sqlite).

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newStepTransitionCarrierTestService extends
// newRunCausationFromTaskTestService's minimal hand-rolled task schema with
// a task_step_transitions table shaped like the production ledger, but
// without its foreign keys — this harness already omits FKs on its other
// hand-rolled tables (see newRunCausationFromTaskTestService's comment).
func newStepTransitionCarrierTestService(t *testing.T) (*Service, *officesqlite.Repository) {
	t.Helper()
	svc, repo := newRunCausationFromTaskTestService(t)
	_, err := repo.ExecRaw(context.Background(), `
		CREATE TABLE IF NOT EXISTS task_step_transitions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			session_id TEXT,
			trigger TEXT NOT NULL,
			actor_kind TEXT NOT NULL,
			actor_id TEXT,
			contract_version INTEGER NOT NULL DEFAULT 1,
			occurred_at TIMESTAMP NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create task_step_transitions: %v", err)
	}
	return svc, repo
}

func seedStepTransition(
	t *testing.T, repo *officesqlite.Repository, id int64, taskID, actorKind, actorID, sessionID string, occurredAt time.Time,
) {
	t.Helper()
	var actorIDArg, sessionIDArg any
	if actorID != "" {
		actorIDArg = actorID
	}
	if sessionID != "" {
		sessionIDArg = sessionID
	}
	_, err := repo.ExecRaw(context.Background(), `
		INSERT INTO task_step_transitions
			(id, task_id, session_id, trigger, actor_kind, actor_id, contract_version, occurred_at)
		VALUES (?, ?, ?, 'test', ?, ?, 1, ?)
	`, id, taskID, sessionIDArg, actorKind, actorIDArg, occurredAt)
	if err != nil {
		t.Fatalf("seed step transition %d: %v", id, err)
	}
}

// mustCreateRunForCarrierTest seeds a queued run directly via the repo,
// bypassing runs/service so the test controls every causation field.
func mustCreateRunForCarrierTest(t *testing.T, repo *officesqlite.Repository, run *models.Run) {
	t.Helper()
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run %q: %v", run.ID, err)
	}
}

// mustClaimRunForCarrierTest transitions a seeded run to claimed at the
// given time — GetRunBySessionAt matches on claimed_at, and SetRunSessionID
// is itself guarded to status='claimed'.
func mustClaimRunForCarrierTest(t *testing.T, repo *officesqlite.Repository, runID string, claimedAt time.Time) {
	t.Helper()
	if err := repo.SetRunStatusForTest(context.Background(), runID, "claimed", &claimedAt, nil); err != nil {
		t.Fatalf("claim run %q: %v", runID, err)
	}
}

// mustSetRunSessionForCarrierTest binds a session id onto an already-claimed
// run, mirroring the production launch write (scheduler_integration.go).
func mustSetRunSessionForCarrierTest(t *testing.T, repo *officesqlite.Repository, runID, sessionID string) {
	t.Helper()
	wrote, err := repo.SetRunSessionID(context.Background(), runID, sessionID)
	if err != nil {
		t.Fatalf("set run session for %q: %v", runID, err)
	}
	if !wrote {
		t.Fatalf("set run session for %q: no row updated (run not claimed?)", runID)
	}
}

// mustFinishRunForCarrierTest moves an already-claimed, session-bound run to
// finished — used by the race-coverage test, where the causing run is
// already terminal by the time the step-entry carrier resolves it.
func mustFinishRunForCarrierTest(t *testing.T, repo *officesqlite.Repository, runID string, claimedAt, finishedAt time.Time) {
	t.Helper()
	if err := repo.SetRunStatusForTest(context.Background(), runID, "finished", &claimedAt, &finishedAt); err != nil {
		t.Fatalf("finish run %q: %v", runID, err)
	}
}

// TestTaskBoundaryCarrierForStepTransition_AgentActor_ResolvesTheClaimedRun
// covers the agent half: a ledger row with actor_kind=agent and a session id
// resolves the run claimed on that session at or before the row's
// occurred_at, and the carrier advances that run's own lineage exactly like
// TaskBoundaryCarrierForRunQueue's live-run preference does.
func TestTaskBoundaryCarrierForStepTransition_AgentActor_ResolvesTheClaimedRun(t *testing.T) {
	svc, repo := newStepTransitionCarrierTestService(t)
	ctx := context.Background()
	seedOfficeTaskWithMetadata(t, repo, "task-1", map[string]interface{}{})

	run := &models.Run{
		ID: "run-1", AgentProfileID: "agent-executor", Reason: "task_assigned",
		Payload: `{"task_id":"task-1"}`, ChainCausationID: "chain-1", CausationDepth: 3,
	}
	mustCreateRunForCarrierTest(t, repo, run)
	claimedAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	mustClaimRunForCarrierTest(t, repo, run.ID, claimedAt)
	mustSetRunSessionForCarrierTest(t, repo, run.ID, "sess-1")

	seedStepTransition(t, repo, 1, "task-1", "agent", "", "sess-1", claimedAt.Add(time.Minute))

	got := svc.TaskBoundaryCarrierForStepTransition(ctx, "task-1", "1")
	if got.ActorKind != models.ActorKindAgent || got.ActorID != "agent-executor" {
		t.Fatalf("actor = %s/%s, want agent/agent-executor", got.ActorKind, got.ActorID)
	}
	if got.CausationID != "chain-1" || got.CreatingRunID != "run-1" || got.CausationDepth != 3 {
		t.Fatalf("lineage = %+v, want carried from run-1", got)
	}
}

// TestTaskBoundaryCarrierForStepTransition_AgentActor_RaceCoverageFinishedRun
// covers the async-dispatch race: the runner's run may already have
// finished by the time DispatchStepEntry's queue_run reaches the office
// carrier resolver, and it must still resolve as the causing run.
func TestTaskBoundaryCarrierForStepTransition_AgentActor_RaceCoverageFinishedRun(t *testing.T) {
	svc, repo := newStepTransitionCarrierTestService(t)
	ctx := context.Background()
	seedOfficeTaskWithMetadata(t, repo, "task-1", map[string]interface{}{})

	run := &models.Run{
		ID: "run-finished", AgentProfileID: "agent-executor", Reason: "task_assigned",
		Payload: `{"task_id":"task-1"}`, ChainCausationID: "chain-2", CausationDepth: 1,
	}
	mustCreateRunForCarrierTest(t, repo, run)
	claimedAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	mustClaimRunForCarrierTest(t, repo, run.ID, claimedAt)
	mustSetRunSessionForCarrierTest(t, repo, run.ID, "sess-2")
	mustFinishRunForCarrierTest(t, repo, run.ID, claimedAt, claimedAt.Add(time.Minute))

	seedStepTransition(t, repo, 2, "task-1", "agent", "", "sess-2", claimedAt.Add(2*time.Minute))

	got := svc.TaskBoundaryCarrierForStepTransition(ctx, "task-1", "2")
	if got.CreatingRunID != "run-finished" {
		t.Fatalf("creating_run_id = %q, want the finished run still resolved", got.CreatingRunID)
	}
}

// TestTaskBoundaryCarrierForStepTransition_HumanActor_ProducesAHumanRootedRoot
// covers the human half: actor_kind=human yields actor_kind=user,
// human_rooted=true, and a fresh root (depth 0) — the move itself is the
// causing event, not any run.
func TestTaskBoundaryCarrierForStepTransition_HumanActor_ProducesAHumanRootedRoot(t *testing.T) {
	svc, repo := newStepTransitionCarrierTestService(t)
	ctx := context.Background()
	seedOfficeTaskWithMetadata(t, repo, "task-1", map[string]interface{}{})
	seedStepTransition(t, repo, 3, "task-1", "human", "user-42", "", time.Now().UTC())

	got := svc.TaskBoundaryCarrierForStepTransition(ctx, "task-1", "3")
	if got.ActorKind != models.ActorKindUser || got.ActorID != "user-42" {
		t.Fatalf("actor = %s/%s, want user/user-42", got.ActorKind, got.ActorID)
	}
	if !got.HumanRooted {
		t.Fatal("human_rooted = false, want true")
	}
	if got.CreatingRunID != "" || got.CausationDepth != 0 {
		t.Fatalf("lineage = %+v, want a fresh root", got)
	}
}

// TestTaskBoundaryCarrierForStepTransition_NoLedgerID_FallsBackUnchanged is
// the regression guard: an empty transition id (every non-step-entry
// trigger) must resolve exactly like the pre-existing TaskBoundaryCarrier,
// not like a malformed or "system" ledger row.
func TestTaskBoundaryCarrierForStepTransition_NoLedgerID_FallsBackUnchanged(t *testing.T) {
	svc, repo := newStepTransitionCarrierTestService(t)
	ctx := context.Background()
	seedOfficeTaskWithMetadata(t, repo, "task-1", map[string]interface{}{})

	got := svc.TaskBoundaryCarrierForStepTransition(ctx, "task-1", "")
	want := svc.TaskBoundaryCarrier(ctx, "task-1")
	if got != want {
		t.Fatalf("carrier = %+v, want unchanged fallback %+v", got, want)
	}
}

// TestTaskBoundaryCarrierForStepTransition_UnknownLedgerID_FallsBackUnchanged
// covers a ledger id that does not resolve to any row (a stale/garbage
// value) — same fallback as the empty case.
func TestTaskBoundaryCarrierForStepTransition_UnknownLedgerID_FallsBackUnchanged(t *testing.T) {
	svc, repo := newStepTransitionCarrierTestService(t)
	ctx := context.Background()
	seedOfficeTaskWithMetadata(t, repo, "task-1", map[string]interface{}{})

	got := svc.TaskBoundaryCarrierForStepTransition(ctx, "task-1", "999999")
	want := svc.TaskBoundaryCarrier(ctx, "task-1")
	if got != want {
		t.Fatalf("carrier = %+v, want unchanged fallback %+v", got, want)
	}
}

// TestTaskBoundaryCarrierForStepTransition_SystemActor_FallsBackUnchanged
// covers a ledger row whose actor_kind is neither agent nor human (system,
// integration, unknown): the step-entry carrier resolution does not apply,
// and the ordinary task-boundary carrier is used instead.
func TestTaskBoundaryCarrierForStepTransition_SystemActor_FallsBackUnchanged(t *testing.T) {
	svc, repo := newStepTransitionCarrierTestService(t)
	ctx := context.Background()
	seedOfficeTaskWithMetadata(t, repo, "task-1", map[string]interface{}{})
	seedStepTransition(t, repo, 4, "task-1", "system", "", "", time.Now().UTC())

	got := svc.TaskBoundaryCarrierForStepTransition(ctx, "task-1", "4")
	want := svc.TaskBoundaryCarrier(ctx, "task-1")
	if got != want {
		t.Fatalf("carrier = %+v, want unchanged fallback %+v", got, want)
	}
}
