package coordinator

import (
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// kindWake seeds an own task with one pending wake of the given kind and
// episode key, leaving the current-episode source for the caller to set.
func (f *deliverFixture) kindWake(t *testing.T, kind WakeKind, key string) (taskID, sessionID string) {
	t.Helper()
	taskID = "tk"
	sessionID = f.ownTask(t, taskID)
	f.conv.tasks[taskID] = &taskmodels.Task{ID: taskID, Identifier: "KAN-tk", Title: "task"}
	insertWakeRow(t, f.store, f.c, "w-tk", taskID, string(kind), key, "pending", time.Now().UTC().Add(-time.Minute))
	return taskID, sessionID
}

func TestDeliver_RechecksEachKindsCurrentEpisode(t *testing.T) {
	stallLast := time.Date(2026, 9, 30, 10, 0, 0, 120_000_000, time.UTC)
	stallKey := stallLast.Format(stallKeyLayout)
	detected := time.Now().UTC().Add(-time.Hour)
	later := detected.Add(time.Minute)

	cases := []struct {
		name     string
		kind     WakeKind
		key      string
		setup    func(f *deliverFixture, taskID, sessionID string)
		wantSent bool
	}{
		{"error stamp unchanged", WakeKindError, "stamp-1", func(f *deliverFixture, _, sid string) {
			f.sources.errStamp[sid] = "stamp-1"
		}, true},
		{"error stamp replaced", WakeKindError, "stamp-1", func(f *deliverFixture, _, sid string) {
			f.sources.errStamp[sid] = "stamp-2"
		}, false},
		{"error cleared", WakeKindError, "stamp-1", func(*deliverFixture, string, string) {}, false},
		{"stall still current", WakeKindStall, stallKey, func(f *deliverFixture, tid, _ string) {
			seedStall(t, f, tid, stallLast, detected)
		}, true},
		{"stall replaced by a newer last_event_at", WakeKindStall, stallKey, func(f *deliverFixture, tid, _ string) {
			seedStall(t, f, tid, stallLast.Add(time.Hour), detected)
		}, false},
		{"stall resumed after detection", WakeKindStall, stallKey, func(f *deliverFixture, tid, _ string) {
			seedStall(t, f, tid, stallLast, detected)
			f.sources.last[tid] = &later
		}, false},
		{"stall row gone", WakeKindStall, stallKey, func(*deliverFixture, string, string) {}, false},
		{"task still completed", WakeKindCompleted, completedEpisodeKey, func(f *deliverFixture, tid, _ string) {
			f.sources.state[tid] = taskStateCompleted
		}, true},
		{"task reopened", WakeKindCompleted, completedEpisodeKey, func(f *deliverFixture, tid, _ string) {
			f.sources.state[tid] = "in_progress"
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeliverFixture(t)
			tid, sid := f.kindWake(t, tc.kind, tc.key)
			tc.setup(f, tid, sid)
			if err := f.deliver(); err != nil {
				t.Fatal(err)
			}
			status, _ := f.wakeStatus(t, "w-tk")
			if tc.wantSent {
				if f.sender.count() != 1 || status != "delivered" {
					t.Fatalf("sent=%d status=%s, want one send and delivered", f.sender.count(), status)
				}
				return
			}
			if f.sender.count() != 0 || len(f.turns(t)) != 0 || status != "superseded" {
				t.Fatalf("sent=%d turns=%d status=%s, want nothing sent and superseded", f.sender.count(), len(f.turns(t)), status)
			}
		})
	}
}

func seedStall(t *testing.T, f *deliverFixture, taskID string, lastEventAt, detectedAt time.Time) {
	t.Helper()
	mustExec(t, f.store, `INSERT INTO coordinator_stalls (task_id, workspace_id, stalled_for_ms, last_event_at, detected_at)
		VALUES (?, ?, ?, ?, ?)`, taskID, f.c.WorkspaceID, 60000, lastEventAt, detectedAt)
}
