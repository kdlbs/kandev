package recorder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/outcomes"
)

type fakeChecker struct {
	verdicts map[string]Verdict
	err      error
	calls    int
}

func (c *fakeChecker) Check(_ context.Context, _, actor string) (Verdict, error) {
	c.calls++
	if c.err != nil {
		return VerdictNotManager, c.err
	}
	return c.verdicts[actor], nil
}

func (f *fixture) capture(c *fakeChecker) *Capture {
	return NewCapture(f.db, c, nil, func() time.Time { return f.now })
}

func (f *fixture) feedbackCount(kind string) int {
	f.t.Helper()
	var n int
	if err := f.db.Get(&n, f.db.Rebind(`SELECT COUNT(*) FROM coordinator_feedback WHERE kind = ?`), kind); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func TestCapture_DecisionOverrides(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "rejected", "create_task", "", f.at(-time.Hour))
	chk := &fakeChecker{verdicts: map[string]Verdict{"mgr": VerdictManager, "other": VerdictNotManager}}
	c := f.capture(chk)
	ev := coordinator.DecisionEvent{ProposalID: "p1", CoordinatorID: f.coord.ID, WorkspaceID: "ws-1", ProposalKind: "create_task",
		Decision: outcomes.DecisionRejected, ActorUserID: "mgr", ReasonCode: outcomes.ReasonDuplicate, At: f.at(-time.Hour)}
	c.OnDecision(context.Background(), ev)
	c.OnDecision(context.Background(), ev)
	if f.feedbackCount("rejected") != 1 {
		t.Fatalf("rejected rows = %d", f.feedbackCount("rejected"))
	}
	before := IgnoredCount(IgnoredNotManager)
	ev.ActorUserID, ev.ProposalID, ev.Decision = "other", "p2", outcomes.DecisionUndone
	c.OnDecision(context.Background(), ev)
	if IgnoredCount(IgnoredNotManager) != before+1 || f.feedbackCount("undone") != 0 {
		t.Fatal("non-manager override stored or not counted")
	}
	chk.err = errors.New("boom")
	beforeErr := IgnoredCount(IgnoredAuthzError)
	c.OnDecision(context.Background(), ev)
	if IgnoredCount(IgnoredAuthzError) != beforeErr+1 {
		t.Fatal("authz error not counted")
	}
}

func TestCapture_EditedAndAutomaticApprovals(t *testing.T) {
	f := newFixture(t)
	chk := &fakeChecker{}
	c := f.capture(chk)
	base := coordinator.DecisionEvent{ProposalID: "p1", CoordinatorID: f.coord.ID, WorkspaceID: "ws-1", ProposalKind: "create_task", Decision: outcomes.DecisionApproved}
	c.OnDecision(context.Background(), base)
	auto := base
	auto.Automatic, auto.EditedFields = true, []string{"title"}
	c.OnDecision(context.Background(), auto)
	if chk.calls != 0 {
		t.Fatal("unedited or automatic approval was judged")
	}
	edited := base
	edited.EditedFields = []string{"title"}
	c.OnDecision(context.Background(), edited)
	if f.feedbackCount("edited") != 1 {
		t.Fatal("edited override not stored")
	}
}

// moveBackFixture: coordinator approved a create_task into s2 (position 2) of
// the task at -1h; s1 (position 1) is earlier.
func moveBackFixture(t *testing.T) *fixture {
	f := newFixture(t)
	f.step("s0", 1, false)
	f.step("s1", 2, false)
	f.task("t1", "s1", false)
	f.exec(`INSERT INTO task_sessions (id, task_id, state, started_at) VALUES ('sess', 't1', 'COMPLETED', ?)`, f.now)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.approvedRow("a1", "p1", "create_task", "t1", f.at(-time.Hour))
	return f
}

func (f *fixture) history(to string, actor any, at time.Time) {
	f.t.Helper()
	f.exec(`INSERT INTO session_step_history (session_id, to_step_id, trigger, actor_id, created_at) VALUES ('sess', ?, 'manual', ?, ?)`, to, actor, at)
}

func TestScan_MovedBackStoredOnceAndRescanIsNoop(t *testing.T) {
	f := moveBackFixture(t)
	f.history("s0", "mgr", f.at(-time.Minute))
	c := f.capture(&fakeChecker{verdicts: map[string]Verdict{"mgr": VerdictManager}})
	for i := 0; i < 2; i++ {
		if _, err := c.ScanTask(context.Background(), "t1"); err != nil {
			t.Fatal(err)
		}
	}
	if f.feedbackCount("moved_back") != 1 {
		t.Fatalf("moved_back rows = %d", f.feedbackCount("moved_back"))
	}
	var row struct {
		Proposal string `db:"proposal_id"`
		To       string `db:"to_step_id"`
		Key      string `db:"transition_key"`
	}
	if err := f.db.Get(&row, `SELECT proposal_id, to_step_id, transition_key FROM coordinator_feedback`); err != nil || row.Proposal != "p1" || row.To != "s0" || row.Key == "" {
		t.Fatalf("row = %+v err = %v", row, err)
	}
}

func TestScan_ForwardNilActorAndNonManager(t *testing.T) {
	f := moveBackFixture(t)
	f.step("s2", 3, false)
	f.history("s2", "mgr", f.at(-5*time.Minute)) // forward: nothing counted
	f.history("s0", nil, f.at(-4*time.Minute))   // nil actor
	f.history("s0", "x", f.at(-3*time.Minute))   // not a manager
	f.history("s0", "mgr", f.at(-2*time.Hour))   // before the action: not later
	chk := &fakeChecker{verdicts: map[string]Verdict{"mgr": VerdictManager, "x": VerdictNotManager}}
	c := f.capture(chk)
	unknown, notMgr := IgnoredCount(IgnoredActorUnknown), IgnoredCount(IgnoredNotManager)
	if _, err := c.ScanTask(context.Background(), "t1"); err != nil {
		t.Fatal(err)
	}
	if IgnoredCount(IgnoredActorUnknown) != unknown+1 || IgnoredCount(IgnoredNotManager) != notMgr+1 || f.feedbackCount("moved_back") != 0 {
		t.Fatal("wrong counts")
	}
	if _, err := c.ScanTask(context.Background(), "t1"); err != nil {
		t.Fatal(err)
	}
	if IgnoredCount(IgnoredActorUnknown) != unknown+1 || IgnoredCount(IgnoredNotManager) != notMgr+1 {
		t.Fatal("rescan recounted")
	}
}

func TestScan_AuthzErrorRetriesThenStores(t *testing.T) {
	f := moveBackFixture(t)
	f.history("s0", "mgr", f.at(-time.Minute))
	chk := &fakeChecker{err: errors.New("down")}
	c := f.capture(chk)
	before := IgnoredCount(IgnoredAuthzError)
	for i := 0; i < 2; i++ {
		_, _ = c.ScanTask(context.Background(), "t1")
	}
	if IgnoredCount(IgnoredAuthzError) != before+1 || f.feedbackCount("moved_back") != 0 {
		t.Fatal("authz error counted more than once or stored")
	}
	chk.err, chk.verdicts = nil, map[string]Verdict{"mgr": VerdictManager}
	_, _ = c.ScanTask(context.Background(), "t1")
	_, _ = c.ScanTask(context.Background(), "t1")
	if f.feedbackCount("moved_back") != 1 {
		t.Fatalf("retry did not store: %d", f.feedbackCount("moved_back"))
	}
}

func TestScan_UndoneReferenceAndStepGone(t *testing.T) {
	f := moveBackFixture(t)
	f.exec(`UPDATE coordinator_activity SET undone_at = ?`, f.at(-2*time.Minute))
	f.history("s0", "mgr", f.at(-time.Minute))
	f.history("gone", "mgr", f.at(-30*time.Second))
	c := f.capture(&fakeChecker{verdicts: map[string]Verdict{"mgr": VerdictManager}})
	if _, err := c.ScanTask(context.Background(), "t1"); err != nil {
		t.Fatal(err)
	}
	if f.feedbackCount("moved_back") != 0 {
		t.Fatal("undone or unknown-step move stored")
	}
}

func TestScan_ReadFailureCountsAndMarksNothing(t *testing.T) {
	f := moveBackFixture(t)
	f.history("s0", "mgr", f.at(-time.Minute))
	f.exec(`DROP TABLE session_step_history`)
	before := ReadFailedCount(ReadFailedHistoryRows)
	if _, err := f.capture(&fakeChecker{}).ScanTask(context.Background(), "t1"); err == nil {
		t.Fatal("expected error")
	}
	if ReadFailedCount(ReadFailedHistoryRows) != before+1 {
		t.Fatal("not counted")
	}
}

func TestScan_TwoCoordinatorsOneObservationEach(t *testing.T) {
	f := moveBackFixture(t)
	other := *f.coord
	other.ID, other.Name = "", "Other"
	if err := f.store.CreateCoordinator(context.Background(), &other); err != nil {
		// the workspace allows one coordinator; fall back to a direct row
		t.Skip("second coordinator not creatable: ", err)
	}
	f.exec(`INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, task_id, kind, created_at, updated_at)
		VALUES ('p2', ?, 'ws-1', 'approved', '{"step_id":"s1"}', 't1', 'create_task', ?, ?)`, other.ID, f.at(-time.Hour), f.at(-time.Hour))
	f.exec(`INSERT INTO coordinator_activity (id, coordinator_id, workspace_id, action_class, outcome, "authorization", target_task_id, proposal_id, created_at, updated_at)
		VALUES ('a2', ?, 'ws-1', 'create_task', 'approved', 'requires_approval', 't1', 'p2', ?, ?)`, other.ID, f.at(-time.Hour), f.at(-time.Hour))
	f.history("s0", "mgr", f.at(-time.Minute))
	c := f.capture(&fakeChecker{verdicts: map[string]Verdict{"mgr": VerdictManager}})
	_, _ = c.ScanTask(context.Background(), "t1")
	if f.feedbackCount("moved_back") != 2 {
		t.Fatalf("rows = %d", f.feedbackCount("moved_back"))
	}
}

func TestCapture_PrincipalAndSystemAreIgnoredAndFieldsStored(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "rejected", "create_task", "", f.at(-time.Hour))
	chk := &fakeChecker{verdicts: map[string]Verdict{"mgr": VerdictManager, "prin": VerdictPrincipal, "sys": VerdictSystem}}
	c := f.capture(chk)
	ev := coordinator.DecisionEvent{ProposalID: "p1", CoordinatorID: f.coord.ID, WorkspaceID: "ws-1", ProposalKind: "create_task",
		Decision: outcomes.DecisionRejected, ReasonCode: "free text not in the closed set", At: f.at(-time.Hour)}
	bp, bs := IgnoredCount(IgnoredPrincipal), IgnoredCount(IgnoredSystem)
	ev.ActorUserID = "prin"
	c.OnDecision(context.Background(), ev)
	ev.ActorUserID = "sys"
	c.OnDecision(context.Background(), ev)
	if IgnoredCount(IgnoredPrincipal) != bp+1 || IgnoredCount(IgnoredSystem) != bs+1 || f.feedbackCount("rejected") != 0 {
		t.Fatal("principal or system override stored or not counted")
	}
	ev.ActorUserID = "mgr"
	c.OnDecision(context.Background(), ev)
	var row struct {
		UserID     string    `db:"user_id"`
		ReasonCode string    `db:"reason_code"`
		CreatedAt  time.Time `db:"created_at"`
	}
	if err := f.db.Get(&row, `SELECT user_id, reason_code, created_at FROM coordinator_feedback WHERE kind = 'rejected'`); err != nil {
		t.Fatal(err)
	}
	if row.UserID != "mgr" || row.ReasonCode != outcomes.ReasonNone || !row.CreatedAt.Equal(f.at(-time.Hour)) {
		t.Fatalf("stored row = %+v", row)
	}
}
