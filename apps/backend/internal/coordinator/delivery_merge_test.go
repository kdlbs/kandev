package coordinator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// mergeWake seeds a watched task the coordinator does not own, an episode of
// the change and a pending wake for that episode.
func (f *deliverFixture) mergeWake(t *testing.T, kind mergestate.Kind) (changeKey string) {
	t.Helper()
	changeKey = "github|github.com|repo-9|42"
	f.svc.merge = true
	f.fix.addTask(t, "tm", "wf-a")
	f.conv.tasks["tm"] = &taskmodels.Task{ID: "tm", Identifier: "KAN-tm", Title: "merge task"}
	in := ChangeObservationInput{WorkspaceID: f.c.WorkspaceID, ChangeKey: changeKey}
	in.Obs = mergeObs(1, "abc123", mergestate.PipelineGreen, mergestate.ConflictClear, 0)
	if _, err := f.store.ApplyChangeObservation(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	in.Obs = mergeObs(2, "abc123", mergestate.PipelineRed, mergestate.ConflictPresent, 2)
	in.Targets = []WakeTarget{{f.c.ID, "tm"}}
	if _, err := f.store.ApplyChangeObservation(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	return changeKey
}

func TestDeliverMergeWakeForUnownedWatchedTask(t *testing.T) {
	f := newDeliverFixture(t)
	f.mergeWake(t, mergestate.KindRed)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 1 {
		var st string
		_ = f.store.db.QueryRow(f.store.db.Rebind(`SELECT status FROM coordinator_wakes LIMIT 1`)).Scan(&st)
		t.Fatalf("sends = %d, want 1; wake status %q; episodes %v", f.sender.count(), st, pendingWakeKinds(t, f.store, f.c.ID))
	}
	msg := f.sender.calls[0].Content
	if !strings.Contains(msg, "change_red") || !strings.Contains(msg, "github") || !strings.Contains(msg, "#42") || !strings.Contains(msg, "abc123") {
		t.Fatalf("message lacks change detail:\n%s", msg)
	}
}

func TestDeliverMergeWakeSupersededWhenEpisodeCleared(t *testing.T) {
	f := newDeliverFixture(t)
	key := f.mergeWake(t, mergestate.KindRed)
	in := ChangeObservationInput{WorkspaceID: f.c.WorkspaceID, ChangeKey: key}
	in.Obs = mergeObs(3, "abc123", mergestate.PipelineGreen, mergestate.ConflictClear, 0)
	if _, err := f.store.ApplyChangeObservation(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 0 {
		t.Fatalf("a cleared red episode was delivered")
	}
	var status string
	if err := f.store.db.QueryRow(f.store.db.Rebind(`SELECT status FROM coordinator_wakes WHERE kind = 'change_red'`)).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "superseded" {
		t.Fatalf("status = %s, want superseded", status)
	}
}

func TestDeliverMergeWakeOutsideWatchIsSuperseded(t *testing.T) {
	f := newDeliverFixture(t)
	f.mergeWake(t, mergestate.KindRed)
	mustExec(t, f.store, `UPDATE coordinators SET watch_scope = 'selected' WHERE id = ?`, f.c.ID)
	mustExec(t, f.store, `INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, 'wf-other', ?, ?)`,
		f.c.ID, f.c.WorkspaceID, time.Now().UTC())
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 0 {
		t.Fatalf("a wake for an unwatched card was delivered")
	}
}

func TestDeliverMergeWakeHeldWhileFlagOff(t *testing.T) {
	f := newDeliverFixture(t)
	f.mergeWake(t, mergestate.KindRed)
	f.svc.merge = false
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 0 {
		t.Fatal("a merge wake was delivered with the flag off")
	}
	var status string
	if err := f.store.db.QueryRow(f.store.db.Rebind(`SELECT status FROM coordinator_wakes WHERE kind = 'change_red'`)).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("status = %s, want the wake retained pending", status)
	}
}
