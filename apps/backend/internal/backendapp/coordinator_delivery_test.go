package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type fakeDeliveryTasks struct {
	primary    *taskmodels.TaskSession
	primaryErr error
	session    *taskmodels.TaskSession
	sessionErr error
	pending    []*taskmodels.Interaction
	turn       *taskmodels.Turn
	turnErr    error
	messages   []*taskmodels.Message
	filters    []taskmodels.PluginMessageFilter
	message    *taskmodels.Message
	updated    *taskmodels.Message
}

func (f *fakeDeliveryTasks) GetPrimarySession(context.Context, string) (*taskmodels.TaskSession, error) {
	return f.primary, f.primaryErr
}

func (f *fakeDeliveryTasks) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	if f.session == nil && f.sessionErr == nil {
		return &taskmodels.TaskSession{}, nil
	}
	return f.session, f.sessionErr
}

func (f *fakeDeliveryTasks) ListPendingInteractions(context.Context, taskmodels.PendingInteractionFilter) ([]*taskmodels.Interaction, error) {
	return f.pending, nil
}

func (f *fakeDeliveryTasks) GetTurn(context.Context, string) (*taskmodels.Turn, error) {
	return f.turn, f.turnErr
}

func (f *fakeDeliveryTasks) ListMessagesForPlugin(_ context.Context, filter taskmodels.PluginMessageFilter) ([]*taskmodels.Message, error) {
	f.filters = append(f.filters, filter)
	start := filter.Offset
	if start > len(f.messages) {
		start = len(f.messages)
	}
	end := start + filter.Limit
	if end > len(f.messages) {
		end = len(f.messages)
	}
	return f.messages[start:end], nil
}

func (f *fakeDeliveryTasks) GetMessage(context.Context, string) (*taskmodels.Message, error) {
	return f.message, nil
}

func (f *fakeDeliveryTasks) UpdateMessage(_ context.Context, m *taskmodels.Message) error {
	f.updated = m
	return nil
}

type fakeQueue struct {
	has bool
	err error
}

func (f fakeQueue) HasPendingForSession(context.Context, string) (bool, error) { return f.has, f.err }

type fakeOrphanCompleter struct {
	got [2]string
	err error
}

func (f *fakeOrphanCompleter) CompleteUnattendedOrphanTurn(_ context.Context, sessionID, turnID string) error {
	f.got = [2]string{sessionID, turnID}
	return f.err
}

func TestConversationReader_PrimarySession(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		tasks   *fakeDeliveryTasks
		want    *coordinator.ConversationSession
		wantErr error
	}{
		{"found", &fakeDeliveryTasks{primary: &taskmodels.TaskSession{ID: "s1", State: taskmodels.TaskSessionStateRunning}}, &coordinator.ConversationSession{ID: "s1", State: "RUNNING"}, nil},
		{"none", &fakeDeliveryTasks{primaryErr: repoerrors.ErrNoPrimarySession}, nil, nil},
		{"missing", &fakeDeliveryTasks{primaryErr: taskmodels.ErrTaskSessionNotFound}, nil, nil},
		{"read error", &fakeDeliveryTasks{primaryErr: boom}, nil, boom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := (&coordinatorConversationReader{tasks: c.tasks}).PrimarySession(context.Background(), "t")
			if !errors.Is(err, c.wantErr) || (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Fatalf("got %+v err %v, want %+v err %v", got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestConversationReader_SessionStateDistinguishesGoneFromError(t *testing.T) {
	boom := errors.New("boom")
	ctx := context.Background()
	state, exists, err := (&coordinatorConversationReader{tasks: &fakeDeliveryTasks{session: &taskmodels.TaskSession{State: taskmodels.TaskSessionStateWaitingForInput}}}).SessionState(ctx, "s")
	if err != nil || !exists || state != "WAITING_FOR_INPUT" {
		t.Fatalf("state=%q exists=%v err=%v", state, exists, err)
	}
	if _, exists, err = (&coordinatorConversationReader{tasks: &fakeDeliveryTasks{sessionErr: taskmodels.ErrTaskSessionNotFound}}).SessionState(ctx, "s"); err != nil || exists {
		t.Fatalf("gone: exists=%v err=%v", exists, err)
	}
	if _, _, err = (&coordinatorConversationReader{tasks: &fakeDeliveryTasks{sessionErr: boom}}).SessionState(ctx, "s"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestConversationReader_ActionPendingAndQueued(t *testing.T) {
	ctx := context.Background()
	r := &coordinatorConversationReader{tasks: &fakeDeliveryTasks{pending: []*taskmodels.Interaction{{ID: "p"}}}, queue: fakeQueue{has: true}}
	if got, err := r.ActionPending(ctx, "s"); err != nil || !got {
		t.Fatalf("ActionPending = %v %v", got, err)
	}
	if got, err := r.Queued(ctx, "s"); err != nil || !got {
		t.Fatalf("Queued = %v %v", got, err)
	}
	empty := &coordinatorConversationReader{tasks: &fakeDeliveryTasks{}, queue: fakeQueue{}}
	if got, _ := empty.ActionPending(ctx, "s"); got {
		t.Fatal("no interactions must not be pending")
	}
	if _, err := (&coordinatorConversationReader{tasks: &fakeDeliveryTasks{}}).Queued(ctx, "s"); err == nil {
		t.Fatal("an unwired queue must fail closed")
	}
}

func TestConversationReader_Turn(t *testing.T) {
	ctx := context.Background()
	done := time.Now()
	if got, err := (&coordinatorConversationReader{tasks: &fakeDeliveryTasks{turn: &taskmodels.Turn{CompletedAt: &done}}}).Turn(ctx, "t"); err != nil || got == nil || !got.Completed {
		t.Fatalf("completed turn = %+v %v", got, err)
	}
	if got, err := (&coordinatorConversationReader{tasks: &fakeDeliveryTasks{turn: &taskmodels.Turn{}}}).Turn(ctx, "t"); err != nil || got == nil || got.Completed {
		t.Fatalf("open turn = %+v %v", got, err)
	}
	if got, err := (&coordinatorConversationReader{tasks: &fakeDeliveryTasks{turnErr: sql.ErrNoRows}}).Turn(ctx, "t"); err != nil || got != nil {
		t.Fatalf("gone turn = %+v %v, want nil", got, err)
	}
	boom := errors.New("boom")
	if _, err := (&coordinatorConversationReader{tasks: &fakeDeliveryTasks{turnErr: boom}}).Turn(ctx, "t"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func wakeMessage(id, turn string, at time.Time) *taskmodels.Message {
	return &taskmodels.Message{ID: id, TurnID: "st-" + id, CreatedAt: at, Metadata: map[string]interface{}{wakeTurnIDKey: turn}}
}

func TestWakeMessages_FindsTheOldestMatchAtOrAfterSinceAcrossPages(t *testing.T) {
	since := time.Now().UTC()
	tasks := &fakeDeliveryTasks{}
	for i := 0; i < wakeMessagePageSize; i++ {
		tasks.messages = append(tasks.messages, wakeMessage("other", "other-turn", since))
	}
	tasks.messages = append(tasks.messages, wakeMessage("early", "ut", since.Add(-time.Second)), wakeMessage("first", "ut", since), wakeMessage("second", "ut", since.Add(time.Second)))
	got, err := (&coordinatorWakeMessages{tasks: tasks}).FindWakeMessage(context.Background(), "s", "ut", since)
	if err != nil || got == nil || got.ID != "first" || got.TurnID != "st-first" {
		t.Fatalf("got %+v err %v, want the first match at or after since", got, err)
	}
	if len(tasks.filters) != 2 || tasks.filters[0].Since == nil || !tasks.filters[0].Since.Equal(since) {
		t.Fatalf("filters = %+v, want two pages bounded by since", tasks.filters)
	}
}

func TestWakeMessages_NoMatchAndGoneSessionReturnNil(t *testing.T) {
	ctx := context.Background()
	since := time.Now()
	none := &fakeDeliveryTasks{messages: []*taskmodels.Message{wakeMessage("m", "other", since)}}
	if got, err := (&coordinatorWakeMessages{tasks: none}).FindWakeMessage(ctx, "s", "ut", since); err != nil || got != nil {
		t.Fatalf("no match: %+v %v", got, err)
	}
	gone := &fakeDeliveryTasks{sessionErr: taskmodels.ErrTaskSessionNotFound}
	if got, err := (&coordinatorWakeMessages{tasks: gone}).FindWakeMessage(ctx, "s", "ut", since); err != nil || got != nil {
		t.Fatalf("gone session: %+v %v", got, err)
	}
	boom := errors.New("boom")
	failing := &fakeDeliveryTasks{sessionErr: boom}
	if _, err := (&coordinatorWakeMessages{tasks: failing}).FindWakeMessage(ctx, "s", "ut", since); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestWakeMessages_MarkOrphanSetsTheFlagAndKeepsTheMetadata(t *testing.T) {
	tasks := &fakeDeliveryTasks{message: &taskmodels.Message{ID: "m", TaskSessionID: "s", Metadata: map[string]interface{}{wakeTurnIDKey: "ut"}}}
	if err := (&coordinatorWakeMessages{tasks: tasks}).MarkOrphan(context.Background(), "s", "m"); err != nil {
		t.Fatal(err)
	}
	if tasks.updated == nil || tasks.updated.Metadata[wakeOrphanedKey] != true || tasks.updated.Metadata[wakeTurnIDKey] != "ut" {
		t.Fatalf("updated = %+v", tasks.updated)
	}
	other := &fakeDeliveryTasks{message: &taskmodels.Message{ID: "m", TaskSessionID: "other"}}
	if err := (&coordinatorWakeMessages{tasks: other}).MarkOrphan(context.Background(), "s", "m"); err == nil || other.updated != nil {
		t.Fatal("a message of another session must not be marked")
	}
}

func TestWakeMessages_CompleteOrphanTurnDelegatesToTheOrchestrator(t *testing.T) {
	orch := &fakeOrphanCompleter{}
	if err := (&coordinatorWakeMessages{orch: orch}).CompleteOrphanTurn(context.Background(), "s", "rt"); err != nil || orch.got != [2]string{"s", "rt"} {
		t.Fatalf("got %v err %v", orch.got, err)
	}
	boom := errors.New("busy")
	if err := (&coordinatorWakeMessages{orch: &fakeOrphanCompleter{err: boom}}).CompleteOrphanTurn(context.Background(), "s", "rt"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the orchestrator's error", err)
	}
	if err := (&coordinatorWakeMessages{}).CompleteOrphanTurn(context.Background(), "s", "rt"); err == nil {
		t.Fatal("an unwired orchestrator must fail closed")
	}
}

func TestRegisterCoordinatorDeliveryWorker_RecoversOpenTurnsAtStartup(t *testing.T) {
	pool := newCoordinatorTestPool(t)
	svc, err := initCoordinatorWiring(context.Background(), pool, newCoordinatorTestTracker(t), nil, nil, nil, true, true, true, false, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	t.Cleanup(svc.StopDelivery)
	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	c := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	started := time.Now().UTC().Add(-time.Hour)
	if _, err := pool.Writer().Exec(`INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at)
		VALUES ('t-open', ?, 'conv', 'sess', 1, 100, ?)`, c.ID, started); err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	hook := registerCoordinatorDeliveryWorker(nil, nil, svc, newTestLogger())
	hook(context.Background(), time.Now())
	var outcome sql.NullString
	if err := pool.Reader().QueryRow(`SELECT outcome FROM coordinator_unattended_turns WHERE id = 't-open'`).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.String != "interrupted" {
		t.Fatalf("outcome = %q, want interrupted", outcome.String)
	}
}
