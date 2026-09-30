package coordinator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type improvementFixture struct {
	*kindsFixture
	cleared []string
}

func newImprovementFixture(t *testing.T) *improvementFixture {
	t.Helper()
	f := &improvementFixture{kindsFixture: proposeFixture(t)}
	f.svc.phase3 = true
	f.svc.validator = newValidatorForTest(
		map[string]*settingsmodels.AgentProfile{"a": {ID: "a", WorkspaceID: "ws-1"}},
		map[string]*taskmodels.ExecutorProfile{"e": {ID: "e"}})
	f.svc.SetConversationHooks(func(_ context.Context, _, oldTaskID string) { f.cleared = append(f.cleared, oldTaskID) }, nil)
	f.tasks.target = &TargetTask{ID: "task-0", WorkspaceID: "ws-1"}
	f.setContext(t, "old instructions")
	f.insertRun(t, "run-1", f.c.ID)
	return f
}

func (f *improvementFixture) setContext(t *testing.T, text string) {
	t.Helper()
	mustExec(t, f.store, `UPDATE coordinators SET context = ? WHERE id = ?`, text, f.c.ID)
}

func (f *improvementFixture) context(t *testing.T) string {
	t.Helper()
	c, err := f.store.GetCoordinatorByID(context.Background(), f.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c.Context
}

func (f *improvementFixture) insertRun(t *testing.T, id, coordinatorID string) {
	t.Helper()
	mustExec(t, f.store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at)
		VALUES (?, ?, 'conv', 'sess', 1, 500, ?)`, id, coordinatorID, time.Now().UTC())
}

func improvementArgs(over map[string]any) json.RawMessage {
	body := map[string]any{
		"title": "Shorter wake prompts", "rationale": "Runs re-read the same context.",
		"context":  "new instructions",
		"evidence": []map[string]string{{"run_id": "run-1"}, {"task_id": "task-0"}},
	}
	for k, v := range over {
		if v == nil {
			delete(body, k)
			continue
		}
		body[k] = v
	}
	raw, _ := json.Marshal(body)
	return raw
}

func (f *improvementFixture) propose(t *testing.T, over map[string]any) (*Proposal, error) {
	t.Helper()
	return f.svc.ProposeImprovement(context.Background(), f.c.ID, improvementArgs(over))
}

func (f *improvementFixture) mustPropose(t *testing.T) *Proposal {
	t.Helper()
	p, err := f.propose(t, nil)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return p
}

// approved returns an improvement proposal approved through the service.
func (f *improvementFixture) approved(t *testing.T) (*Proposal, *PendingChange) {
	t.Helper()
	p := f.mustPropose(t)
	got, err := f.approve(p, nil)
	if err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("approve: got=%+v err=%v", got, err)
	}
	changes, err := f.svc.ListPendingChanges(context.Background(), "ws-1", f.c.ID)
	if err != nil || len(changes) == 0 {
		t.Fatalf("list changes: %v %v", changes, err)
	}
	return got, changes[len(changes)-1]
}

func (f *improvementFixture) apply(changeID string) (*Coordinator, error) {
	return f.svc.ApplyPendingChange(context.Background(), "ws-1", f.c.ID, changeID)
}

func (f *improvementFixture) proposalCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.store.db.Get(&n, `SELECT COUNT(*) FROM coordinator_proposals WHERE kind = 'improvement'`); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *improvementFixture) changeRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.store.db.Get(&n, `SELECT COUNT(*) FROM coordinator_pending_changes`); err != nil {
		t.Fatal(err)
	}
	return n
}

func jsonUnmarshal(raw string, into any) error { return json.Unmarshal([]byte(raw), into) }

func timeNow() time.Time { return time.Now() }
