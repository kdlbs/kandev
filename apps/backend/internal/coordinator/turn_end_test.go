package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeFinder struct {
	msgs        map[string]*WakeMessage
	findErr     error
	completeErr error
	marked      [][2]string
	completed   [][2]string
}

func (f *fakeFinder) FindWakeMessage(_ context.Context, _, wakeTurnID string, _ time.Time) (*WakeMessage, error) {
	return f.msgs[wakeTurnID], f.findErr
}

func (f *fakeFinder) MarkOrphan(_ context.Context, sessionID, messageID string) error {
	f.marked = append(f.marked, [2]string{sessionID, messageID})
	return nil
}

func (f *fakeFinder) CompleteOrphanTurn(_ context.Context, sessionID, turnID string) error {
	f.completed = append(f.completed, [2]string{sessionID, turnID})
	return f.completeErr
}

type openTurn struct {
	id, sessionTurn, reserved, message string
	started                            time.Time
	stopRequested                      bool
}

// openTurnRow inserts an open turn with one delivered wake.
func (f *deliverFixture) openTurnRow(t *testing.T, o openTurn) {
	t.Helper()
	nullable := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	var stop any
	if o.stopRequested {
		stop = time.Now().UTC()
	}
	if o.started.IsZero() {
		o.started = time.Now().UTC().Add(-10 * time.Minute)
	}
	mustExec(t, f.store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, message_id, session_turn_id, reserved_turn_id, stop_requested_at, wake_count, start_ceiling_subcents, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, 500, ?)`,
		o.id, f.c.ID, ceilingConvTask, ceilingSession, nullable(o.message), nullable(o.sessionTurn), nullable(o.reserved), stop, o.started)
	insertWakeRow(t, f.store, f.c, "w-"+o.id, "task-"+o.id, string(WakeKindQuestion), "q-"+o.id, "delivered", o.started)
	mustExec(t, f.store, `UPDATE coordinator_wakes SET turn_id = ? WHERE id = ?`, o.id, "w-"+o.id)
}

func (f *deliverFixture) turnRow(t *testing.T, id string) (outcome sql.NullString, finished sql.NullTime, cost, message sql.NullString) {
	t.Helper()
	var c sql.NullInt64
	err := f.store.db.QueryRow(f.store.db.Rebind(`SELECT outcome, finished_at, cost_subcents, message_id FROM coordinator_unattended_turns WHERE id = ?`), id).
		Scan(&outcome, &finished, &c, &message)
	if err != nil {
		t.Fatal(err)
	}
	if c.Valid {
		cost.Valid = true
		cost.String = fmt.Sprint(c.Int64)
	}
	return outcome, finished, cost, message
}

func completedEvent(turnID, sessionID string) *bus.Event {
	return bus.NewEvent(events.TurnCompleted, "test", map[string]any{"id": turnID, "session_id": sessionID})
}

func (f *deliverFixture) endTurn(sessionTurnID string) {
	f.svc.onTurnCompleted(context.Background(), completedEvent(sessionTurnID, ceilingSession))
}

func TestTurnEnd_SettlesCompletedWithCostCountAndPublish(t *testing.T) {
	f := newDeliverFixture(t)
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	before := expvarMapValue(unattendedTurnTotal, outcomeCompleted)
	f.endTurn("st")
	outcome, finished, cost, _ := f.turnRow(t, "ut")
	if outcome.String != outcomeCompleted || !finished.Valid || cost.String != "55" {
		t.Fatalf("row = %v %v %v, want completed with cost 55", outcome, finished, cost)
	}
	if got := expvarMapValue(unattendedTurnTotal, outcomeCompleted); got != before+1 {
		t.Fatalf("counter = %d, want %d", got, before+1)
	}
	seq := f.seq()
	if len(seq) < 2 || seq[len(seq)-2] != "updated" || seq[len(seq)-1] != "kick:"+f.c.ID {
		t.Fatalf("events = %v, want update then kick", seq)
	}
	if status, _ := f.wakeStatus(t, "w-ut"); status != "delivered" {
		t.Fatalf("wake = %s, want delivered", status)
	}
}

func TestTurnEnd_OutcomeFollowsTheSessionState(t *testing.T) {
	cases := []struct {
		name  string
		state taskmodels.TaskSessionState
		gone  bool
		stop  bool
		want  string
	}{
		{"idle", taskmodels.TaskSessionStateWaitingForInput, false, false, outcomeCompleted},
		{"running drained queue", taskmodels.TaskSessionStateRunning, false, false, outcomeCompleted},
		{"failed", taskmodels.TaskSessionStateFailed, false, false, outcomeFailed},
		{"cancelled", taskmodels.TaskSessionStateCancelled, false, false, outcomeCancelled},
		{"session gone", "", true, false, outcomeCancelled},
		{"stop requested beats failed", taskmodels.TaskSessionStateFailed, false, true, outcomeStopped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeliverFixture(t)
			f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st", stopRequested: tc.stop})
			if tc.gone {
				delete(f.reader.states, ceilingSession)
			} else {
				f.reader.states[ceilingSession] = string(tc.state)
			}
			f.endTurn("st")
			if outcome, _, _, _ := f.turnRow(t, "ut"); outcome.String != tc.want {
				t.Fatalf("outcome = %q, want %q", outcome.String, tc.want)
			}
		})
	}
}

func TestTurnEnd_ADuplicateEventDoesNotCountOrPublishAgain(t *testing.T) {
	f := newDeliverFixture(t)
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	f.endTurn("st")
	before := len(f.seq())
	count := expvarMapValue(unattendedTurnTotal, outcomeCompleted)
	f.endTurn("st")
	if got := expvarMapValue(unattendedTurnTotal, outcomeCompleted); got != count {
		t.Fatalf("counter moved %d -> %d", count, got)
	}
	if got := len(f.seq()); got != before {
		t.Fatalf("a duplicate event published %d more events", got-before)
	}
}

func TestTurnEnd_AnUnboundTurnIsNotSettledByAnotherTurnsCompletion(t *testing.T) {
	f := newDeliverFixture(t)
	f.openTurnRow(t, openTurn{id: "ut", reserved: "rt"})
	f.endTurn("rt")
	f.endTurn("other")
	if outcome, _, _, _ := f.turnRow(t, "ut"); outcome.Valid {
		t.Fatalf("outcome = %v, want the unbound turn left open", outcome)
	}
}

func TestTurnEnd_AFailedSessionReadLeavesTheRowOpen(t *testing.T) {
	f := newDeliverFixture(t)
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	f.reader.stateErr = errors.New("boom")
	f.endTurn("st")
	if outcome, _, _, _ := f.turnRow(t, "ut"); outcome.Valid {
		t.Fatalf("outcome = %v, want open", outcome)
	}
}

func TestTurnEnd_ACostErrorNeverBlocksTheSettle(t *testing.T) {
	f := newDeliverFixture(t)
	f.ledger.turnFn = func(turnCall) (int64, error) { return 0, errors.New("boom") }
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	f.endTurn("st")
	outcome, _, cost, _ := f.turnRow(t, "ut")
	if outcome.String != outcomeCompleted || cost.Valid {
		t.Fatalf("row = %v cost %v, want completed with null cost", outcome, cost)
	}
}
