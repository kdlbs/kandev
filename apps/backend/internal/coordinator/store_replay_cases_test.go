package coordinator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

func execAll(t *testing.T, s *Store, q string, args ...any) {
	t.Helper()
	if _, err := s.db.Exec(s.db.Rebind(q), args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func seedReplayTurn(t *testing.T, s *Store, id string, started time.Time, decision string, feedback bool) {
	t.Helper()
	execAll(t, s, `INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", wake_kinds, snapshot_hash, started_at)
		VALUES (?, 'c1', 'sess', ?, 'message', '["stall"]', ?, ?)`, id, "st-"+id, "h-"+id, started.UTC())
	execAll(t, s, `INSERT INTO coordinator_outcomes (proposal_id, coordinator_id, turn_id, kind, decision, edited_fields, decided_at, graded_at)
		VALUES (?, 'c1', ?, 'message', ?, '["title"]', ?, ?)`, "p-"+id, id, decision, started.UTC(), started.UTC())
	if feedback {
		execAll(t, s, `INSERT INTO coordinator_feedback (id, coordinator_id, kind, proposal_id, turn_id, user_id, created_at)
			VALUES (?, 'c1', 'override', ?, ?, 'u', ?)`, "f-"+id, "p-"+id, id, started.UTC())
	}
}

func TestReplayCases_SelectTurnsGroupsAndOrder(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := range 55 {
		seedReplayTurn(t, s, fmt.Sprintf("a%02d", i), now.Add(-time.Duration(i+1)*time.Hour), replay.DecisionApproved, false)
	}
	seedReplayTurn(t, s, "undecided", now.Add(-time.Minute), "pending", false)
	seedReplayTurn(t, s, "ov-old", now.Add(-100*24*time.Hour), replay.DecisionRejected, true)
	seedReplayTurn(t, s, "ov-in", now.Add(-30*24*time.Hour), replay.DecisionRejected, true)
	seedReplayTurn(t, s, "ov-recent", now.Add(-10*time.Minute), replay.DecisionReturned, true)

	turns, err := NewReplayCases(s).SelectTurns(context.Background(), "c1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 51 {
		t.Fatalf("turns = %d, want 50 in group one + 1 override turn", len(turns))
	}
	if turns[0].ID != "ov-recent" || turns[1].ID != "a00" {
		t.Fatalf("group one starts %s, %s", turns[0].ID, turns[1].ID)
	}
	last := turns[len(turns)-1]
	if last.ID != "ov-in" || turns[49].ID != "a48" {
		t.Fatalf("order: turns[49]=%s last=%s", turns[49].ID, last.ID)
	}
	for _, tn := range turns {
		if tn.ID == "undecided" || tn.ID == "ov-old" {
			t.Fatalf("turn %s must not be selected", tn.ID)
		}
	}
	if o := turns[1].Outcomes; len(o) != 1 || o[0].ProposalID != "p-a00" || o[0].EditedFields[0] != "title" || turns[1].WakeKinds[0] != "stall" {
		t.Fatalf("turn data = %+v", turns[1])
	}
}

func TestReplayCases_TiesBreakByTurnIDDescendingInBothGroups(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := range 48 {
		seedReplayTurn(t, s, fmt.Sprintf("a%02d", i), now.Add(-time.Duration(i+2)*time.Hour), replay.DecisionApproved, false)
	}
	for _, id := range []string{"b1", "b2"} {
		seedReplayTurn(t, s, id, now.Add(-time.Hour), replay.DecisionApproved, false)
	}
	for _, id := range []string{"o1", "o2", "o3"} {
		seedReplayTurn(t, s, id, now.Add(-30*24*time.Hour), replay.DecisionRejected, true)
	}
	turns, err := NewReplayCases(s).SelectTurns(context.Background(), "c1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 53 {
		t.Fatalf("turns = %d", len(turns))
	}
	var got []string
	for _, i := range []int{0, 1, 2, 49, 50, 51, 52} {
		got = append(got, turns[i].ID)
	}
	want := []string{"b2", "b1", "a00", "a47", "o3", "o2", "o1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("order %v, want %v", got, want)
	}
}

func TestReplayCases_ReadsAndNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := NewReplayCases(s)
	execAll(t, s, `CREATE TABLE tasks (id TEXT PRIMARY KEY, title TEXT NOT NULL)`)
	execAll(t, s, `CREATE TABLE task_session_messages (id TEXT PRIMARY KEY, task_session_id TEXT, turn_id TEXT, author_type TEXT, content TEXT, created_at TIMESTAMP)`)
	execAll(t, s, `INSERT INTO tasks VALUES ('t1', '')`)
	now := time.Now().UTC()
	seedReplayTurn(t, s, "turn1", now, replay.DecisionApproved, false)
	execAll(t, s, `INSERT INTO task_session_messages VALUES ('m2','sess','st-turn1','user','second',?)`, now.Add(time.Second))
	execAll(t, s, `INSERT INTO task_session_messages VALUES ('m1','sess','st-turn1','user','first',?)`, now)
	execAll(t, s, `INSERT INTO task_session_messages VALUES ('m0','sess','st-turn1','agent','agent text',?)`, now.Add(-time.Second))
	execAll(t, s, `INSERT INTO coordinator_turn_snapshots (hash, body, created_at) VALUES ('h-turn1','body',?)`, now)
	execAll(t, s, `INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at, kind, target_task_id)
		VALUES ('p-turn1','c1','w','approved','{"title":"Do it","workflow_id":"wf"}',?,?,'create_task',NULL)`, now, now)

	if got, err := c.TriggerText(ctx, replay.Turn{ID: "turn1"}); err != nil || got != "first" {
		t.Fatalf("trigger = %q %v", got, err)
	}
	if got, err := c.TaskTitle(ctx, "t1"); err != nil || got != "" {
		t.Fatalf("present empty title = %q %v", got, err)
	}
	if got, err := c.Snapshot(ctx, "h-turn1"); err != nil || got != "body" {
		t.Fatalf("snapshot = %q %v", got, err)
	}
	p, err := c.Proposal(ctx, "p-turn1")
	if err != nil || p != (replay.Proposal{Kind: "create_task", WorkflowID: "wf", Title: "Do it"}) {
		t.Fatalf("proposal = %+v %v", p, err)
	}
	if _, err := c.TriggerText(ctx, replay.Turn{ID: "nope"}); !errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("missing trigger = %v", err)
	}
	if _, err := c.TaskTitle(ctx, "gone"); !errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("missing title = %v", err)
	}
	if _, err := c.Snapshot(ctx, "x"); !errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("missing snapshot = %v", err)
	}
	if _, err := c.Proposal(ctx, "x"); !errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("missing proposal = %v", err)
	}
	execAll(t, s, `DROP TABLE tasks`)
	if _, err := c.TaskTitle(ctx, "t1"); err == nil || errors.Is(err, replay.ErrNotFound) {
		t.Fatalf("unreadable table must be a failed read, got %v", err)
	}
}
