package wakeup

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

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

	d := NewDispatcher(repo, repo, logger.Default())
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
