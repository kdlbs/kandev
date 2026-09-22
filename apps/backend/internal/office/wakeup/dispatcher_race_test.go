package wakeup

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// raceTestRunQueuer adapts a runs/service.Service directly to
// wakeup.RunQueuer, bypassing office/service.Service: this file is an
// internal (white-box) wakeup test (needs the unexported
// coalesceIntoInflightRun), and office/service now imports office/wakeup
// (pause-gate integration), so importing office/service here would be a
// test-only import cycle. The dispatcher-level guard office/service.Service
// adds (agent status) is irrelevant to what this test exercises. Mirrors
// office/service.Service.QueueRunFromWakeup's shape.
type raceTestRunQueuer struct {
	svc *runsservice.Service
}

func (q *raceTestRunQueuer) QueueRunFromWakeup(
	ctx context.Context, agentProfileID, reason, routineID, contextSnapshot, causationID, idempotencyKey string,
) (string, error) {
	outcome, row, err := q.svc.QueueRunAndReturn(ctx, runsservice.QueueRunRequest{
		Reason:          reason,
		Payload:         map[string]any{"agent_profile_id": agentProfileID},
		ActorKind:       officemodels.ActorKindSystem,
		RoutineID:       routineID,
		ContextSnapshot: contextSnapshot,
		SkipCoalesce:    true,
		CausationID:     causationID,
		IdempotencyKey:  idempotencyKey,
	})
	if err != nil {
		return "", err
	}
	if row == nil {
		return "", fmt.Errorf("queue run from wakeup: no row returned for agent %s (outcome %s)", agentProfileID, outcome)
	}
	return row.ID, nil
}

// TestCoalesceIntoInflightRun_ClaimedBetweenReadAndPromote pins the
// TOCTOU this package's atomic promotion and coalesce transaction exists
// to close: coalesceIntoInflightRun must consult the affected-row count
// from the conditional UPDATE, not the in-memory Status captured by the
// earlier FindInflightRunForAgent read.
//
// Reverting coalesceIntoInflightRun to a stale in-memory check (`if
// inflight.Status != "queued" { createFreshRun }` followed by an
// unconditional reason write) passes every other dispatcher test,
// because they all seed the run already `claimed` or already `queued`
// with no interleaving — the in-memory status and the row's current
// status always agree. This test forces them to disagree: it captures
// `inflight` while the row is still queued, then claims the row (as
// the scheduler would, concurrently) before invoking the unexported
// coalesce path with the now-stale copy.
func TestCoalesceIntoInflightRun_ClaimedBetweenReadAndPromote(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	agent := &officemodels.AgentInstance{
		ID:                    "agent-1",
		WorkspaceID:           "ws-1",
		Name:                  "ceo",
		AgentDisplayName:      "CEO",
		Role:                  officemodels.AgentRoleCEO,
		Status:                officemodels.AgentStatusIdle,
		MaxConcurrentSessions: 1,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	run := &officemodels.Run{
		ID:              "run-cron",
		AgentProfileID:  agent.ID,
		Reason:          shared.RunReasonRoutineDispatchCron,
		Payload:         "{}",
		Status:          officemodels.RunStatus("queued"),
		CoalescedCount:  1,
		ContextSnapshot: `{"prior":"snapshot"}`,
	}
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	req := &officesqlite.WakeupRequest{
		ID: "w-event", AgentProfileID: agent.ID, Source: SourceRoutine,
		Payload: `{"routine_id":"r-1"}`, Reason: shared.RunReasonRoutineDispatchEvent,
	}
	if err := repo.CreateWakeupRequest(context.Background(), req); err != nil {
		t.Fatalf("seed wakeup: %v", err)
	}

	// Dispatcher's real Dispatch() reads the in-flight run here, while
	// the row is still queued. Capture that same stale snapshot.
	inflight, err := repo.FindInflightRunForAgent(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("find inflight: %v", err)
	}
	if inflight.Status != officemodels.RunStatus("queued") {
		t.Fatalf("precondition: inflight.Status = %q, want queued", inflight.Status)
	}

	// The scheduler races in and claims the row before the dispatcher's
	// promotion write lands.
	claimed, err := repo.ClaimNextEligibleRun(context.Background())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != run.ID {
		t.Fatalf("claim: got %q, want %q", claimed.ID, run.ID)
	}

	log := logger.Default()
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	d := NewDispatcher(repo, repo, log)
	d.SetRunQueuer(&raceTestRunQueuer{svc: runsSvc})
	if err := d.coalesceIntoInflightRun(context.Background(), req, inflight); err != nil {
		t.Fatalf("coalesceIntoInflightRun: %v", err)
	}

	gotReq, err := repo.GetWakeupRequest(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	if gotReq.Status != officesqlite.WakeupStatusClaimed {
		t.Errorf("status: got %q, want claimed (own run, not coalesced into the claimed one)", gotReq.Status)
	}
	if gotReq.RunID == "" || gotReq.RunID == run.ID {
		t.Errorf("expected a fresh run distinct from %q, got %q", run.ID, gotReq.RunID)
	}

	claimedRun, err := repo.GetRunByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get claimed run: %v", err)
	}
	if claimedRun.Reason != shared.RunReasonRoutineDispatchCron {
		t.Errorf("claimed run.Reason = %q, want unchanged %q — promoting it here would race "+
			"the idle-skip decision the scheduler already started", claimedRun.Reason, shared.RunReasonRoutineDispatchCron)
	}
}

// TestCreateFreshRun_RetryDedupesOntoTheSameRunInsteadOfOrphaning pins the
// fix for the enqueue-then-claim gap: createFreshRun's QueueRunFromWakeup
// call and its MarkWakeupRequestClaimed call are two separate writes with
// no shared transaction. If MarkWakeupRequestClaimed fails (or two
// dispatchers race the same request), a naive retry of createFreshRun
// would call QueueRunFromWakeup a second time and mint a second, orphaned
// run for the same wakeup-request. wakeupRunIdempotencyKey(req.ID) closes
// that: the second call's IdempotencyKey collides with the first run's,
// so runs/service dedupes it (QueueOutcomeDeduped, no new row) instead of
// creating a duplicate.
func TestCreateFreshRun_RetryDedupesOntoTheSameRunInsteadOfOrphaning(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	agent := &officemodels.AgentInstance{
		ID:               "agent-1",
		WorkspaceID:      "ws-1",
		Name:             "ceo",
		AgentDisplayName: "CEO",
		Role:             officemodels.AgentRoleCEO,
		Status:           officemodels.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	req := &officesqlite.WakeupRequest{
		ID: "w-retry", AgentProfileID: agent.ID, Source: SourceSelf,
	}
	if err := repo.CreateWakeupRequest(context.Background(), req); err != nil {
		t.Fatalf("seed wakeup: %v", err)
	}

	log := logger.Default()
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	d := NewDispatcher(repo, repo, log)
	d.SetRunQueuer(&raceTestRunQueuer{svc: runsSvc})

	if err := d.createFreshRun(context.Background(), req); err != nil {
		t.Fatalf("first createFreshRun: %v", err)
	}
	firstReq, err := repo.GetWakeupRequest(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("get wakeup request after first call: %v", err)
	}
	if firstReq.RunID == "" {
		t.Fatalf("expected a run id after the first call")
	}

	// A retry against the still-queued request (as if the first call's
	// MarkWakeupRequestClaimed had failed after QueueRunFromWakeup
	// succeeded) must not mint a second run.
	if err := d.createFreshRun(context.Background(), req); err == nil {
		t.Fatalf("expected the retried createFreshRun to fail (deduped enqueue, no row to claim against), got nil error")
	}

	var runCount int
	if err := db.Get(&runCount, "SELECT COUNT(*) FROM runs WHERE agent_profile_id = ?", agent.ID); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("run count = %d, want 1 (retry must dedupe onto the existing run, not orphan a second one)", runCount)
	}
}
