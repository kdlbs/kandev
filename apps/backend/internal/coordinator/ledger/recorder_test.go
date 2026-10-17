package ledger

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
)

func TestRecorder_StartWritesOneStampedRow(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`UPDATE coordinators SET config_revision = 7 WHERE id = ?`, f.coord.ID)
	f.start("st-1", s)
	f.start("st-1", s) // redelivered start

	row := f.oneTurn()
	if row.Trigger != TriggerMessage || row.Agent != "agent-1" || row.PromptHash != "prompt-hash" {
		t.Fatalf("row = %+v", row)
	}
	if row.Model != "" || row.Harness != "" || row.FinishedAt != nil {
		t.Fatalf("model/harness/finished must be empty at start: %+v", row)
	}
	if row.SnapshotHash == "" {
		t.Fatal("snapshot hash empty")
	}
	if !row.StartedAt.Equal(s) {
		t.Fatalf("started_at = %v, want the event's %v", row.StartedAt, s)
	}
	if row.WatchScope != "all" || row.WatchIDs != "[]" {
		t.Fatalf("watch = %q %q", row.WatchScope, row.WatchIDs)
	}
	if row.ConfigRevision != 7 || row.PolicyRevision != int64(f.coord.PolicyRevision) {
		t.Fatalf("revisions = %d/%d, want 7/%d", row.ConfigRevision, row.PolicyRevision, f.coord.PolicyRevision)
	}
	if got := f.l.ActiveTurnID(testSession); got != row.ID {
		t.Fatalf("ActiveTurnID = %q, want %q", got, row.ID)
	}
}

func TestRecorder_IgnoresNonCoordinatorSessions(t *testing.T) {
	f := newFixture(t)
	ev := turnEventData("st-1", f.at(0), nil)
	ev.Data.(map[string]any)["task_id"] = "other-task"
	f.l.OnTurnStarted(t.Context(), ev)
	if n := len(f.turns()); n != 0 {
		t.Fatalf("rows = %d, want 0", n)
	}
}

func TestRecorder_CompletionIsIdempotentUnderConcurrency(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.start("st-1", s)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'WAITING_FOR_INPUT')`, testSession)
	f.exec(`INSERT INTO task_usage_events (session_id, turn_id, agent_type, model, created_at) VALUES (?, 'st-1', 'claude-acp', '  Opus-X ', ?)`, testSession, s)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.complete("st-1", s, s.Add(time.Second))
		}()
	}
	wg.Wait()

	row := f.oneTurn()
	if row.FinishedAt == nil || row.Outcome == nil || *row.Outcome != "completed" || row.Verdict == nil || *row.Verdict != VerdictNothingNeeded {
		t.Fatalf("row = %+v", row)
	}
	if row.Model != "opus-x" || row.Harness != "claude-acp@v9" {
		t.Fatalf("model/harness = %q/%q", row.Model, row.Harness)
	}
	if got := f.l.ActiveTurnID(testSession); got != "" {
		t.Fatalf("ActiveTurnID after completion = %q", got)
	}
}

func TestRecorder_FirstCompletionWinsAndModelSetOnce(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.start("st-1", s)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'FAILED')`, testSession)
	f.complete("st-1", s, s.Add(time.Second))
	f.exec(`UPDATE task_sessions SET state = 'IDLE'`)
	f.exec(`INSERT INTO task_usage_events (session_id, turn_id, agent_type, model, created_at) VALUES (?, 'st-1', 'a', 'late-model', ?)`, testSession, s)
	f.complete("st-1", s, s.Add(2*time.Second))

	row := f.oneTurn()
	if *row.Outcome != "failed" || *row.Verdict != VerdictBlocked {
		t.Fatalf("first completion must win: %+v", row)
	}
	if row.Model != "" {
		t.Fatalf("a no-op second completion must not fill the model: %q", row.Model)
	}
	if got := row.FinishedAt.Sub(s); got != time.Second {
		t.Fatalf("finished_at moved by second completion: %v", got)
	}
}

func TestRecorder_CompletionBeforeStartLeavesFinishedRow(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`INSERT INTO task_session_turns (id, started_at) VALUES ('st-1', ?)`, s)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'COMPLETED')`, testSession)
	f.complete("st-1", s.Add(time.Minute), s.Add(2*time.Minute)) // payload start differs; the session turn row wins
	f.start("st-1", s)                                           // late start must not touch the row

	row := f.oneTurn()
	if row.FinishedAt == nil || row.SnapshotHash != "" {
		t.Fatalf("late row: %+v", row)
	}
	if !row.StartedAt.Equal(s) {
		t.Fatalf("late row started_at = %v, want the session turn row's %v", row.StartedAt, s)
	}
	if got := f.l.ActiveTurnID(testSession); got != "" {
		t.Fatalf("finished row left an active entry %q", got)
	}
}

func TestRecorder_AttendedUnreadableStateIsUnknownNeverBlocked(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.start("st-1", s)
	f.complete("st-1", s, s.Add(time.Second)) // no task_sessions row

	row := f.oneTurn()
	if *row.Outcome != "unknown" || *row.Verdict != VerdictNothingNeeded {
		t.Fatalf("row = %+v", row)
	}
}

func TestRecorder_StartStampsSelectedWatchScope(t *testing.T) {
	f := newFixture(t)
	f.watch = coordinator.WatchSet{WorkflowIDs: []string{"wf-2", "wf-1"}}
	f.start("st-1", f.at(0))
	row := f.oneTurn()
	if row.WatchScope != "workflows" || row.WatchIDs != `["wf-1","wf-2"]` {
		t.Fatalf("watch = %q %q", row.WatchScope, row.WatchIDs)
	}
}

func TestRecorder_DreamEpisodeTurnIsStampedWithTheDreamTrigger(t *testing.T) {
	f := newFixture(t)
	f.l.deps.IsDreamTask = func(_ context.Context, taskID string) (bool, error) { return taskID == testConvTask, nil }
	f.start("st-dream", f.at(0))

	if row := f.oneTurn(); row.Trigger != TriggerDream {
		t.Fatalf("trigger = %q, want %q", row.Trigger, TriggerDream)
	}
}
