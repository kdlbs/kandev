package ledger

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
)

func (f *fixture) task(id, workflow, step, state string, updated time.Time, archived bool) {
	f.t.Helper()
	var archivedAt any
	if archived {
		archivedAt = updated
	}
	f.exec(`INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, state, archived_at, updated_at) VALUES (?, 'ws-1', ?, ?, ?, ?, ?)`,
		id, workflow, step, state, archivedAt, updated)
}

func (f *fixture) snapshot(started time.Time) (*Snapshot, snapshotBody) {
	f.t.Helper()
	snap, err := BuildSnapshot(f.t.Context(), f.db, f.coord.ID, "ws-1", f.watch, started)
	if err != nil {
		f.t.Fatal(err)
	}
	var body snapshotBody
	if err := json.Unmarshal(snap.Body, &body); err != nil {
		f.t.Fatal(err)
	}
	return snap, body
}

func TestSnapshot_HashIsStableAndOrderIndependent(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	f.task("t-b", "wf", "s-open", "TODO", f.at(time.Minute), false)
	f.task("t-a", "wf", "s-open", "TODO", f.at(time.Minute), false)
	a, _ := f.snapshot(f.at(time.Hour))
	b, _ := f.snapshot(f.at(2 * time.Hour))
	if a.Hash != b.Hash || string(a.Body) != string(b.Body) {
		t.Fatal("same board produced two snapshots")
	}
	f.exec(`UPDATE tasks SET state = 'IN_PROGRESS' WHERE id = 't-a'`)
	if c, _ := f.snapshot(f.at(time.Hour)); c.Hash == a.Hash {
		t.Fatal("a changed board kept its hash")
	}
}

func TestSnapshot_SelectsOpenWatchedTasksNewestFirstWithTiesById(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0), ('s-done', 1)`)
	f.task("t-old", "wf", "s-open", "TODO", f.at(0), false)
	f.task("t-b", "wf", "s-open", "TODO", f.at(time.Minute), false)
	f.task("t-a", "wf", "s-open", "TODO", f.at(time.Minute), false)
	f.task("t-archived", "wf", "s-open", "TODO", f.at(time.Hour), true)
	f.task("t-done", "wf", "s-done", "COMPLETED", f.at(time.Hour), false)
	f.task("t-unwatched", "wf-other", "s-open", "TODO", f.at(time.Hour), false)
	f.watch = coordinator.WatchSet{WorkflowIDs: []string{"wf"}}

	_, body := f.snapshot(f.at(time.Hour))
	var ids []string
	for _, tk := range body.Tasks {
		ids = append(ids, tk.TaskID)
	}
	if fmt.Sprint(ids) != "[t-a t-b t-old]" || body.TasksTotal != 3 {
		t.Fatalf("tasks = %v total %d", ids, body.TasksTotal)
	}
}

func TestSnapshot_EmptyWatchSetHoldsNoTasks(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	f.task("t1", "wf", "s-open", "TODO", f.at(0), false)
	f.watch = coordinator.WatchSet{}
	if _, body := f.snapshot(f.at(time.Hour)); len(body.Tasks) != 0 || body.TasksTotal != 0 {
		t.Fatalf("body = %+v", body)
	}
}

func TestSnapshot_CapsCarryTheirTotals(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	for i := 0; i < snapshotTaskCap+5; i++ {
		f.task(fmt.Sprintf("t%03d", i), "wf", "s-open", "TODO", f.at(time.Duration(i)*time.Second), false)
	}
	for i := 0; i < snapshotProposalCap+3; i++ {
		f.proposal(fmt.Sprintf("p%03d", i), f.coord.ID, f.at(-time.Duration(i+1)*time.Second))
	}
	_, body := f.snapshot(f.at(time.Hour))
	if len(body.Tasks) != snapshotTaskCap || body.TasksTotal != snapshotTaskCap+5 ||
		len(body.Proposals) != snapshotProposalCap || body.ProposalsTotal != snapshotProposalCap+3 || !body.Truncated {
		t.Fatalf("caps: tasks %d/%d proposals %d/%d truncated %v", len(body.Tasks), body.TasksTotal, len(body.Proposals), body.ProposalsTotal, body.Truncated)
	}
	if body.Proposals[0].ID != fmt.Sprintf("p%03d", snapshotProposalCap+2) {
		t.Fatalf("proposals not oldest first: %s", body.Proposals[0].ID)
	}
}

func TestSnapshot_ProposalsOfTheTurnItselfAreLeftOutAndKindsAreSorted(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	f.task("t1", "wf", "s-open", "TODO", f.at(0), false)
	started := f.at(time.Hour)
	for i, p := range []struct {
		id, kind, status, target string
		created                  time.Time
	}{
		{"p-move", "move", "pending", "t1", started.Add(-time.Minute)},
		{"p-msg", "message", "pending", "t1", started.Add(-time.Minute)},
		{"p-failed", "resume", "failed", "t1", started.Add(-time.Minute)},
		{"p-decided", "message", "approved", "t1", started.Add(-time.Minute)},
		{"p-own", "resume", "pending", "t2", started},
	} {
		f.proposal(p.id, f.coord.ID, p.created)
		f.exec(`UPDATE coordinator_proposals SET kind = ?, status = ?, target_task_id = ? WHERE id = ?`, p.kind, p.status, p.target, p.id)
		_ = i
	}
	_, body := f.snapshot(started)
	if len(body.Proposals) != 3 {
		t.Fatalf("proposals = %+v", body.Proposals)
	}
	if fmt.Sprint(body.Tasks[0].Kinds) != "[message move]" {
		t.Fatalf("kinds = %v (pending proposals only, sorted)", body.Tasks[0].Kinds)
	}
}

func TestSnapshot_StoredOnceAcrossTurnsAndNamedByTheRow(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	f.task("t1", "wf", "s-open", "TODO", f.at(0), false)
	f.start("st-1", f.at(time.Hour))
	f.start("st-2", f.at(2*time.Hour))
	rows := f.turns()
	if len(rows) != 2 || rows[0].SnapshotHash == "" || rows[0].SnapshotHash != rows[1].SnapshotHash {
		t.Fatalf("rows = %+v", rows)
	}
	var n int
	if err := f.db.Get(&n, `SELECT COUNT(*) FROM coordinator_turn_snapshots`); err != nil || n != 1 {
		t.Fatalf("snapshots = %d, err %v", n, err)
	}
}

func TestSnapshot_FailedBuildLeavesEmptyHashAndRowWritten(t *testing.T) {
	f := newFixture(t)
	f.exec(`DROP TABLE tasks`)
	before := FailureCount(StageSnapshot)
	f.start("st-1", f.at(0))
	if row := f.oneTurn(); row.SnapshotHash != "" {
		t.Fatalf("hash = %q", row.SnapshotHash)
	}
	if FailureCount(StageSnapshot) != before+1 {
		t.Fatal("snapshot failure not counted")
	}
}

func TestSnapshot_FailedStoreLeavesEmptyHashAndRowWritten(t *testing.T) {
	f := newFixture(t)
	f.exec(`DROP TABLE coordinator_turn_snapshots`)
	before := FailureCount(StageSnapshot)
	f.start("st-1", f.at(0))
	row := f.oneTurn()
	if row.SnapshotHash != "" {
		t.Fatalf("hash = %q, want empty", row.SnapshotHash)
	}
	if FailureCount(StageSnapshot) != before+1 {
		t.Fatal("snapshot store failure not counted")
	}
	if f.l.ActiveTurnID(testSession) == "" {
		t.Fatal("active turn entry was dropped")
	}
}

func TestSnapshot_BodyHoldsOnlyAllowedKeysAndNoTaskOrProposalText(t *testing.T) {
	f := newFixture(t)
	f.exec(`ALTER TABLE tasks ADD COLUMN title TEXT`)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	f.task("t1", "wf", "s-open", "TODO", f.at(0), false)
	f.exec(`UPDATE tasks SET title = 'SECRET-TITLE' WHERE id = 't1'`)
	f.proposal("p1", f.coord.ID, f.at(-time.Minute))
	f.exec(`UPDATE coordinator_proposals SET spec_json = '{"text":"SECRET-SPEC"}', target_task_id = 't1' WHERE id = 'p1'`)

	snap, _ := f.snapshot(f.at(time.Hour))
	if strings.Contains(string(snap.Body), "SECRET") {
		t.Fatalf("snapshot leaks task or proposal text: %s", snap.Body)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(snap.Body, &top); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, "body", top, "proposals", "proposals_total", "tasks", "tasks_total")
	var tasks, proposals []map[string]json.RawMessage
	_ = json.Unmarshal(top["tasks"], &tasks)
	_ = json.Unmarshal(top["proposals"], &proposals)
	assertKeys(t, "task", tasks[0], "kinds", "state", "step_id", "task_id", "updated_at")
	assertKeys(t, "proposal", proposals[0], "id", "kind", "status", "target_task_id")
}

func assertKeys(t *testing.T, what string, got map[string]json.RawMessage, want ...string) {
	t.Helper()
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sort.Strings(want)
	if fmt.Sprint(keys) != fmt.Sprint(want) {
		t.Fatalf("%s keys = %v, want %v", what, keys, want)
	}
}

func TestSnapshot_NullUpdatedAtSortsLastAndIsRecordedNull(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	f.task("t-dated", "wf", "s-open", "TODO", f.at(0), false)
	f.exec(`INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, state, updated_at) VALUES ('t-null', 'ws-1', 'wf', 's-open', 'TODO', NULL)`)
	_, body := f.snapshot(f.at(time.Hour))
	if len(body.Tasks) != 2 || body.Tasks[0].TaskID != "t-dated" || body.Tasks[1].TaskID != "t-null" {
		t.Fatalf("tasks = %+v", body.Tasks)
	}
	if body.Tasks[1].UpdatedAt != nil || body.Tasks[0].UpdatedAt == nil {
		t.Fatalf("updated_at = %v / %v", body.Tasks[0].UpdatedAt, body.Tasks[1].UpdatedAt)
	}
}

func TestSnapshot_ProposalEntriesCarryIdKindStatusAndTarget(t *testing.T) {
	f := newFixture(t)
	f.proposal("p-b", f.coord.ID, f.at(-time.Minute))
	f.proposal("p-a", f.coord.ID, f.at(-time.Minute))
	f.proposal("p-none", f.coord.ID, f.at(-time.Minute))
	f.exec(`UPDATE coordinator_proposals SET kind = 'move', status = 'approving', target_task_id = 't9' WHERE id = 'p-a'`)
	f.exec(`UPDATE coordinator_proposals SET kind = 'message', status = 'failed', target_task_id = 't8' WHERE id = 'p-b'`)
	f.exec(`UPDATE coordinator_proposals SET target_task_id = NULL WHERE id = 'p-none'`)
	_, body := f.snapshot(f.at(time.Hour))
	if len(body.Proposals) != 3 {
		t.Fatalf("proposals = %+v", body.Proposals)
	}
	want := []struct {
		id, kind, status, target string
	}{{"p-a", "move", "approving", "t9"}, {"p-b", "message", "failed", "t8"}, {"p-none", "create_task", "pending", ""}}
	for i, w := range want {
		p := body.Proposals[i]
		got := ""
		if p.TargetTaskID != nil {
			got = *p.TargetTaskID
		}
		if p.ID != w.id || p.Kind != w.kind || p.Status != w.status || got != w.target {
			t.Fatalf("proposal %d = %+v, want %+v", i, p, w)
		}
	}
	if body.Proposals[2].TargetTaskID != nil {
		t.Fatal("a proposal without a target must encode null")
	}
}

func TestSnapshot_NoOrphanWhenTheRowInsertFails(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	f.task("t1", "wf", "s-open", "TODO", f.at(0), false)
	f.exec(`DROP TABLE coordinator_turns`)
	f.start("st-1", f.at(time.Hour))
	var n int
	if err := f.db.Get(&n, `SELECT COUNT(*) FROM coordinator_turn_snapshots`); err != nil || n != 0 {
		t.Fatalf("snapshots after a failed row insert = %d, err %v", n, err)
	}
}

func TestSnapshot_TaskKindsIncludeProposalsBeyondTheProposalCap(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO workflow_steps (id, complete_task_on_enter) VALUES ('s-open', 0)`)
	f.task("t1", "wf", "s-open", "TODO", f.at(0), false)
	started := f.at(time.Hour)
	for i := 0; i < snapshotProposalCap; i++ {
		f.proposal(fmt.Sprintf("old%03d", i), f.coord.ID, started.Add(-time.Duration(i+10)*time.Minute))
	}
	f.proposal("late", f.coord.ID, started.Add(-time.Minute))
	f.exec(`UPDATE coordinator_proposals SET kind = 'resume', status = 'pending', target_task_id = 't1' WHERE id = 'late'`)
	_, body := f.snapshot(started)
	if len(body.Proposals) != snapshotProposalCap || body.ProposalsTotal != snapshotProposalCap+1 {
		t.Fatalf("proposals %d/%d", len(body.Proposals), body.ProposalsTotal)
	}
	for _, p := range body.Proposals {
		if p.ID == "late" {
			t.Fatal("late proposal inside the cap; test does not exercise it")
		}
	}
	if fmt.Sprint(body.Tasks[0].Kinds) != "[resume]" {
		t.Fatalf("kinds = %v, want [resume]", body.Tasks[0].Kinds)
	}
}
