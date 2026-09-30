package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeSender struct {
	mu        sync.Mutex
	calls     []orchestrator.UnattendedWakePrompt
	err       error
	reserve   string
	accept    string
	messageID string
}

func (f *fakeSender) PromptUnattendedWake(_ context.Context, in orchestrator.UnattendedWakePrompt) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, in)
	err, reserve, accept, id := f.err, f.reserve, f.accept, f.messageID
	f.mu.Unlock()
	if reserve != "" && in.OnReserved != nil {
		in.OnReserved(reserve)
	}
	if err != nil {
		return "", err
	}
	if accept != "" && in.OnAccepted != nil {
		in.OnAccepted(accept)
	}
	return id, nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// hookSources runs onRead before every question read.
type hookSources struct {
	*fakeWakeSources
	onRead func()
}

func (h *hookSources) PendingQuestionID(ctx context.Context, sessionID string) (string, error) {
	if h.onRead != nil {
		h.onRead()
	}
	return h.fakeWakeSources.PendingQuestionID(ctx, sessionID)
}

type deliverFixture struct {
	*wakeEnv
	conv      *fakeConversationTasks
	reader    *fakeConvReader
	sender    *fakeSender
	hooks     *hookSources
	ledger    *fakeSpendLedger
	active    *fakeActiveTurns
	canceller *fakeCanceller
	c         *Coordinator
}

func newDeliverFixture(t *testing.T) *deliverFixture {
	t.Helper()
	env := newWakeEnv(t)
	f := &deliverFixture{wakeEnv: env, conv: newFakeConversationTasks(), sender: &fakeSender{messageID: "msg-1"}, c: env.fix.c}
	f.conv.tasks[ceilingConvTask] = spendConvTask(ceilingConvTask, f.c.ID, false)
	ledger := &fakeSpendLedger{}
	ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{}, nil }
	ledger.turnFn = func(turnCall) (int64, error) { return 55, nil }
	f.svc.conversationTasks = f.conv
	f.ledger, f.active, f.canceller = ledger, &fakeActiveTurns{turns: map[string]string{}}, &fakeCanceller{}
	f.svc.SetSpendDeps(ledger, f.active, f.canceller)
	mustExec(t, f.store, `UPDATE coordinators SET cost_ceiling_subcents = 1000, conversation_task_id = ? WHERE id = ?`, ceilingConvTask, f.c.ID)
	f.reader = &fakeConvReader{
		primary: &ConversationSession{ID: ceilingSession, State: string(taskmodels.TaskSessionStateWaitingForInput)},
		states:  map[string]string{ceilingSession: string(taskmodels.TaskSessionStateWaitingForInput)},
	}
	f.svc.SetDeliveryDeps(f.reader, nil, f.sender)
	f.svc.SetContainment(newChecker(contained()))
	f.hooks = &hookSources{fakeWakeSources: env.sources}
	f.svc.wakeSources = f.hooks
	return f
}

// wake adds an own task with a live question episode and a pending wake for it.
func (f *deliverFixture) wake(t *testing.T, id, title string, at time.Time) {
	t.Helper()
	sid := f.ownTask(t, id)
	f.sources.mu.Lock()
	f.sources.question[sid] = "q-" + id
	f.sources.mu.Unlock()
	f.conv.tasks[id] = &taskmodels.Task{ID: id, Identifier: "KAN-" + id, Title: title}
	insertWakeRow(t, f.store, f.c, "w-"+id, id, string(WakeKindQuestion), "q-"+id, "pending", at)
}

func insertWakeRow(t *testing.T, s *Store, c *Coordinator, id, taskID, kind, key, status string, at time.Time) {
	t.Helper()
	mustExec(t, s, `INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, c.ID, c.WorkspaceID, taskID, kind, key, status, at, at)
}

func (f *deliverFixture) deliver() error {
	return f.svc.Deliver(context.Background(), f.c.ID)
}

func (f *deliverFixture) wakeStatus(t *testing.T, id string) (status string, turnID sql.NullString) {
	t.Helper()
	err := f.store.db.QueryRow(f.store.db.Rebind(`SELECT status, turn_id FROM coordinator_wakes WHERE id = ?`), id).Scan(&status, &turnID)
	if err != nil {
		t.Fatal(err)
	}
	return status, turnID
}

type turnSnapshot struct {
	ID, SessionID, ConvTask                   string
	MessageID, SessionTurn, Reserved, Outcome sql.NullString
	WakeCount                                 int
	StartCeiling                              int64
}

func (f *deliverFixture) turns(t *testing.T) []turnSnapshot {
	t.Helper()
	rows, err := f.store.db.Query(`SELECT id, session_id, conversation_task_id, message_id, session_turn_id, reserved_turn_id, outcome, wake_count, start_ceiling_subcents
		FROM coordinator_unattended_turns ORDER BY started_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []turnSnapshot
	for rows.Next() {
		var s turnSnapshot
		if err := rows.Scan(&s.ID, &s.SessionID, &s.ConvTask, &s.MessageID, &s.SessionTurn, &s.Reserved, &s.Outcome, &s.WakeCount, &s.StartCeiling); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestDeliver_HeldAdmissionSendsNothingAndCounts(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.reader.states[ceilingSession] = string(taskmodels.TaskSessionStateRunning)
	f.reader.primary.State = string(taskmodels.TaskSessionStateRunning)
	before := expvarMapValue(admissionHeldTotal, admitConvBusy)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 0 || len(f.turns(t)) != 0 {
		t.Fatalf("a held admission sent %d and created %d turns", f.sender.count(), len(f.turns(t)))
	}
	if got := expvarMapValue(admissionHeldTotal, admitConvBusy); got != before+1 {
		t.Fatalf("held counter = %d, want %d", got, before+1)
	}
}

func TestDeliver_SendsOneTurnForTheHoldingWakes(t *testing.T) {
	f := newDeliverFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	f.wake(t, "t1", "first", base)
	f.wake(t, "t2", "second", base.Add(time.Minute))
	f.sender.reserve, f.sender.accept = "rt-1", "st-1"
	delivered := wakeDeliveredTotal.Value()
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	turns := f.turns(t)
	if len(turns) != 1 || f.sender.count() != 1 {
		t.Fatalf("turns=%d sends=%d, want 1 and 1", len(turns), f.sender.count())
	}
	tr := turns[0]
	if tr.WakeCount != 2 || tr.StartCeiling != 1000 || tr.SessionID != ceilingSession || tr.ConvTask != ceilingConvTask {
		t.Fatalf("turn = %+v", tr)
	}
	if tr.MessageID.String != "msg-1" || tr.Reserved.String != "rt-1" || tr.SessionTurn.String != "st-1" {
		t.Fatalf("turn bindings = %+v", tr)
	}
	call := f.sender.calls[0]
	if call.WakeTurnID != tr.ID || call.TaskID != ceilingConvTask || call.SessionID != ceilingSession {
		t.Fatalf("send = %+v", call)
	}
	if !strings.Contains(call.Content, "Events since your last turn (2):\n- question on KAN-t1 \"first\"\n- question on KAN-t2 \"second\"\n") {
		t.Fatalf("content = %q", call.Content)
	}
	for _, id := range []string{"w-t1", "w-t2"} {
		if status, turn := f.wakeStatus(t, id); status != "delivered" || turn.String != tr.ID {
			t.Fatalf("wake %s = %s/%v", id, status, turn)
		}
	}
	if got := wakeDeliveredTotal.Value(); got != delivered+2 {
		t.Fatalf("delivered counter = %d, want %d", got, delivered+2)
	}
	if got := f.seq(); len(got) == 0 || got[len(got)-1] != "updated" {
		t.Fatalf("events = %v, want a trailing autonomy update", got)
	}
}

func TestDeliver_SupersedesAnEndedEpisodeAndSendsNothingWhenNoneHolds(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.sources.mu.Lock()
	f.sources.question["s-t1"] = "q-changed"
	f.sources.mu.Unlock()
	before := wakeSupersededTotal.Value()
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "superseded" {
		t.Fatalf("status = %s, want superseded", status)
	}
	if f.sender.count() != 0 || len(f.turns(t)) != 0 {
		t.Fatal("a turn was created with no holding wake")
	}
	if got := wakeSupersededTotal.Value(); got != before+1 {
		t.Fatalf("superseded counter = %d, want %d", got, before+1)
	}
	if got := f.seq(); len(got) == 0 || got[len(got)-1] != "updated" {
		t.Fatalf("events = %v, want an autonomy update", got)
	}
}

func TestDeliver_SupersedesAWakeWhoseTaskIsNoLongerOwned(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	mustExec(t, f.store, `UPDATE coordinator_proposals SET status = 'rejected'`)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "superseded" {
		t.Fatalf("status = %s, want superseded", status)
	}
}

func TestDeliver_KeepsTheFirstTwentyAndLeavesTheRestPending(t *testing.T) {
	f := newDeliverFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 25; i++ {
		f.wake(t, fmt.Sprintf("t%02d", i), "x", base.Add(time.Duration(i)*time.Second))
	}
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if got := f.turns(t)[0].WakeCount; got != 20 {
		t.Fatalf("wake_count = %d, want 20", got)
	}
	if status, _ := f.wakeStatus(t, "w-t19"); status != "delivered" {
		t.Fatalf("w-t19 = %s, want delivered", status)
	}
	if status, _ := f.wakeStatus(t, "w-t20"); status != "pending" {
		t.Fatalf("w-t20 = %s, want pending", status)
	}
}

func TestDeliver_AWakeReadErrorLeavesThatWakePendingAndExcluded(t *testing.T) {
	f := newDeliverFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	f.wake(t, "t1", "one", base)
	f.wake(t, "t2", "two", base.Add(time.Minute))
	f.sources.failWith("PendingQuestionID", errBoom)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	f.sources.failWith("PendingQuestionID", nil)
	if f.sender.count() != 0 {
		t.Fatal("sent with every wake read failing")
	}
	for _, id := range []string{"w-t1", "w-t2"} {
		if status, _ := f.wakeStatus(t, id); status != "pending" {
			t.Fatalf("wake %s = %s, want pending", id, status)
		}
	}
}

func TestDeliver_AFailedWatchSetReadAbortsBeforeAnySupersede(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.sources.mu.Lock()
	f.sources.question["s-t1"] = "q-changed"
	f.sources.mu.Unlock()
	mustExec(t, f.store, `UPDATE coordinators SET watch_scope = 'selected' WHERE id = ?`, f.c.ID)
	mustExec(t, f.store, `INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, 'wf-a', ?, ?)`,
		f.c.ID, f.c.WorkspaceID, time.Now().UTC())
	mustExec(t, f.store, `DROP TABLE workflows`)
	if err := f.deliver(); err == nil {
		t.Fatal("want the watch set read error")
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "pending" {
		t.Fatalf("status = %s, want pending", status)
	}
}

func TestDeliver_AutonomyOffBetweenAdmissionAndTheTransactionRollsBack(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.hooks.onRead = func() { setAutonomy(t, f.store, f.c.ID, false) }
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 0 || len(f.turns(t)) != 0 {
		t.Fatal("delivered after autonomy went off")
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "pending" {
		t.Fatalf("status = %s, want pending", status)
	}
}

func TestDeliver_AnOpenTurnCreatedAfterAdmissionRollsBack(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.hooks.onRead = func() {
		mustExec(t, f.store, `INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at)
			VALUES ('other', ?, ?, ?, 1, 500, ?)`, f.c.ID, ceilingConvTask, ceilingSession, time.Now().UTC())
	}
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 0 {
		t.Fatal("sent over another delivery's open turn")
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "pending" {
		t.Fatalf("status = %s, want pending", status)
	}
}

func TestDeliver_ANotDispatchedSendSettlesSendFailedAndReturnsTheWakes(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.sender.err = fmt.Errorf("%w: %w", orchestrator.ErrWakePromptNotDispatched, errors.New("busy"))
	settled := expvarMapValue(unattendedTurnTotal, "send_failed")
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	tr := f.turns(t)[0]
	if tr.Outcome.String != "send_failed" || tr.MessageID.Valid {
		t.Fatalf("turn = %+v, want send_failed with no message", tr)
	}
	if status, turn := f.wakeStatus(t, "w-t1"); status != "pending" || turn.Valid {
		t.Fatalf("wake = %s/%v, want pending with no turn", status, turn)
	}
	if got := expvarMapValue(unattendedTurnTotal, "send_failed"); got != settled+1 {
		t.Fatalf("settle counter = %d, want %d", got, settled+1)
	}
}

func TestDeliver_AnAmbiguousSendErrorLeavesTheTurnOpen(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.sender.err = context.DeadlineExceeded
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	tr := f.turns(t)[0]
	if tr.Outcome.Valid {
		t.Fatalf("outcome = %v, want the turn left open", tr.Outcome)
	}
	if status, turn := f.wakeStatus(t, "w-t1"); status != "delivered" || turn.String != tr.ID {
		t.Fatalf("wake = %s/%v, want delivered", status, turn)
	}
}

func TestDeliver_ConcurrentDeliversSendOnce(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC())
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.deliver()
		}()
	}
	wg.Wait()
	if got := f.sender.count(); got != 1 {
		t.Fatalf("sends = %d, want 1", got)
	}
	if got := len(f.turns(t)); got != 1 {
		t.Fatalf("turn rows = %d, want 1", got)
	}
}
