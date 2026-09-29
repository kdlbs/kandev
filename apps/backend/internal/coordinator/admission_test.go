package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeConvReader struct {
	primary    *ConversationSession
	primaryErr error
	states     map[string]string
	stateErr   error
	pending    bool
	pendingErr error
	queued     bool
	queuedErr  error
}

func (f *fakeConvReader) PrimarySession(context.Context, string) (*ConversationSession, error) {
	return f.primary, f.primaryErr
}

func (f *fakeConvReader) SessionState(_ context.Context, id string) (string, bool, error) {
	st, ok := f.states[id]
	return st, ok, f.stateErr
}

func (f *fakeConvReader) ActionPending(context.Context, string) (bool, error) {
	return f.pending, f.pendingErr
}

func (f *fakeConvReader) Queued(context.Context, string) (bool, error) {
	return f.queued, f.queuedErr
}

type admitFixture struct {
	*ceilingFixture
	reader *fakeConvReader
	fc     *fakeContainment
}

func newAdmitFixture(t *testing.T) *admitFixture {
	t.Helper()
	f := &admitFixture{ceilingFixture: newCeilingFixture(t, 1000), fc: contained()}
	mustExec(t, f.env.store, `DELETE FROM coordinator_unattended_turns`)
	mustExec(t, f.env.store, `UPDATE coordinators SET autonomy_enabled = ?, conversation_task_id = ? WHERE id = ?`,
		true, ceilingConvTask, f.env.c.ID)
	f.reader = &fakeConvReader{
		primary: &ConversationSession{ID: ceilingSession, State: string(taskmodels.TaskSessionStateWaitingForInput)},
		states:  map[string]string{ceilingSession: string(taskmodels.TaskSessionStateWaitingForInput)},
	}
	f.turns.turns = map[string]string{}
	f.env.svc.SetDeliveryDeps(f.reader, nil, nil)
	f.env.svc.SetContainment(newChecker(f.fc))
	return f
}

func (f *admitFixture) admit(mode AdmitMode) Admission {
	return f.env.svc.Admit(context.Background(), f.env.c.ID, mode)
}

func (f *admitFixture) setState(state taskmodels.TaskSessionState) {
	f.reader.primary.State = string(state)
	f.reader.states[ceilingSession] = string(state)
}

func wantHeld(t *testing.T, got Admission, reason, detail string) {
	t.Helper()
	if got.OK || got.Reason != reason || got.Detail != detail {
		t.Fatalf("admission = %+v, want held %q/%q", got, reason, detail)
	}
}

func TestAdmit_AllChecksPass(t *testing.T) {
	f := newAdmitFixture(t)
	if got := f.admit(AdmitCounting); !got.OK {
		t.Fatalf("admission = %+v, want ok", got)
	}
}

func TestAdmit_UnsetModeAndMissingCoordinator(t *testing.T) {
	f := newAdmitFixture(t)
	wantHeld(t, f.env.svc.Admit(context.Background(), f.env.c.ID, 0), "autonomy_off", "read_error")
	wantHeld(t, f.env.svc.Admit(context.Background(), "nope", AdmitCounting), "autonomy_off", "coordinator_not_found")
}

func TestAdmit_AutonomyOff(t *testing.T) {
	f := newAdmitFixture(t)
	mustExec(t, f.env.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, false, f.env.c.ID)
	wantHeld(t, f.admit(AdmitCounting), "autonomy_off", "")
}

func TestAdmit_ContainmentNamesTheFailingCondition(t *testing.T) {
	f := newAdmitFixture(t)
	f.fc.mode = "disabled"
	got := f.admit(AdmitCounting)
	if got.OK || got.Reason != "containment" || got.Detail == "" {
		t.Fatalf("admission = %+v, want containment with a condition", got)
	}
}

func TestAdmit_ReadOnlyModeDoesNotCountContainment(t *testing.T) {
	f := newAdmitFixture(t)
	f.fc.mode = "disabled"
	before := containmentFailedCount("auth_enabled")
	f.admit(AdmitReadOnly)
	if got := containmentFailedCount("auth_enabled"); got != before {
		t.Fatalf("read-only admit moved the counter %d -> %d", before, got)
	}
	f.admit(AdmitCounting)
	if got := containmentFailedCount("auth_enabled"); got != before+1 {
		t.Fatalf("counting admit counter = %d, want %d", got, before+1)
	}
}

func TestAdmit_SpendUnmeasuredAndCeiling(t *testing.T) {
	f := newAdmitFixture(t)
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{}, errors.New("boom") }
	wantHeld(t, f.admit(AdmitCounting), "spend_unmeasured", "")
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{CostSubcents: 1000}, nil }
	wantHeld(t, f.admit(AdmitCounting), "ceiling_reached", "")
}

func TestAdmit_Conversation(t *testing.T) {
	f := newAdmitFixture(t)
	f.conv.tasks[ceilingConvTask].ArchivedAt = &time.Time{}
	wantHeld(t, f.admit(AdmitCounting), "no_conversation", "")
	delete(f.conv.tasks, ceilingConvTask)
	wantHeld(t, f.admit(AdmitCounting), "no_conversation", "")
	mustExec(t, f.env.store, `UPDATE coordinators SET conversation_task_id = NULL WHERE id = ?`, f.env.c.ID)
	wantHeld(t, f.admit(AdmitCounting), "no_conversation", "")
}

func TestAdmit_ConversationUnavailable(t *testing.T) {
	f := newAdmitFixture(t)
	for _, st := range []taskmodels.TaskSessionState{taskmodels.TaskSessionStateFailed, taskmodels.TaskSessionStateCancelled, taskmodels.TaskSessionStateCompleted} {
		f.setState(st)
		wantHeld(t, f.admit(AdmitCounting), "conversation_unavailable", "")
	}
	f.setState(taskmodels.TaskSessionStateCreated)
	wantHeld(t, f.admit(AdmitCounting), "conversation_unavailable", "session_not_started")
	f.reader.primary = nil
	wantHeld(t, f.admit(AdmitCounting), "conversation_unavailable", "")
}

func TestAdmit_ConversationBusy(t *testing.T) {
	f := newAdmitFixture(t)
	f.setState(taskmodels.TaskSessionStateRunning)
	wantHeld(t, f.admit(AdmitCounting), "conversation_busy", "")
	f.setState(taskmodels.TaskSessionStateIdle)
	if got := f.admit(AdmitCounting); !got.OK {
		t.Fatalf("IDLE = %+v, want ok", got)
	}
	f.reader.pending = true
	wantHeld(t, f.admit(AdmitCounting), "conversation_busy", "")
	f.reader.pending = false
	f.reader.queued = true
	wantHeld(t, f.admit(AdmitCounting), "conversation_busy", "")
	f.reader.queued = false
	f.turns.turns[ceilingSession] = "active"
	wantHeld(t, f.admit(AdmitCounting), "conversation_busy", "")
	delete(f.turns.turns, ceilingSession)
	f.insertTurn(t, "open", ceilingSession, "", 500)
	wantHeld(t, f.admit(AdmitCounting), "conversation_busy", "turn_open")
}

func TestAdmit_ConversationBusyReadErrorsFailClosed(t *testing.T) {
	f := newAdmitFixture(t)
	f.reader.pendingErr = errors.New("boom")
	wantHeld(t, f.admit(AdmitCounting), "conversation_busy", "read_error")
	f.reader.pendingErr = nil
	f.reader.queuedErr = errors.New("boom")
	wantHeld(t, f.admit(AdmitCounting), "conversation_busy", "read_error")
	f.reader.queuedErr = nil
	f.turns.err = errors.New("boom")
	wantHeld(t, f.admit(AdmitCounting), "conversation_busy", "read_error")
	f.turns.err = nil
	f.reader.primaryErr = errors.New("boom")
	wantHeld(t, f.admit(AdmitCounting), "conversation_unavailable", "read_error")
}

func TestAdmit_CooldownIsInclusiveAtFiveMinutes(t *testing.T) {
	f := newAdmitFixture(t)
	finished := f.env.store.now().Add(-4 * time.Minute)
	mustExec(t, f.env.store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at, outcome, finished_at)
		VALUES ('done', ?, ?, ?, 1, 500, ?, 'completed', ?)`,
		f.env.c.ID, ceilingConvTask, ceilingSession, finished.Add(-time.Minute), finished)
	got := f.admit(AdmitCounting)
	wantHeld(t, got, "cooldown", "")
	if got.Until == nil || !got.Until.Equal(finished.Add(5*time.Minute)) {
		t.Fatalf("until = %v, want %v", got.Until, finished.Add(5*time.Minute))
	}
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET finished_at = ? WHERE id = 'done'`,
		f.env.store.now().Add(-5*time.Minute))
	if got := f.admit(AdmitCounting); !got.OK {
		t.Fatalf("at 5m = %+v, want ok", got)
	}
}
