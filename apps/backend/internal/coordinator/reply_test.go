package coordinator

import (
	"context"
	"errors"
	"expvar"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type fakeReplyMessenger struct {
	mu      sync.Mutex
	err     error
	calls   int
	ids     []string
	reqs    []*taskservice.CreateMessageRequest
	queued  []*messagequeue.QueuedMessage
	stored  map[string]bool
	created []bool
}

func (f *fakeReplyMessenger) CreateQueuedMessageOnce(_ context.Context, id string, req *taskservice.CreateMessageRequest,
	q *messagequeue.QueuedMessage, _ int) (*taskmodels.Message, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, false, f.err
	}
	if f.stored == nil {
		f.stored = map[string]bool{}
	}
	created := !f.stored[id]
	f.stored[id] = true
	f.ids, f.reqs, f.queued, f.created = append(f.ids, id), append(f.reqs, req), append(f.queued, q), append(f.created, created)
	return &taskmodels.Message{ID: id, TaskSessionID: req.TaskSessionID}, created, nil
}

type fakeReplyNotifier struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeReplyNotifier) MaxQueuedPromptsPerSession() int { return 10 }
func (f *fakeReplyNotifier) NotifyQueuedUserPrompt(_ context.Context, taskID, sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, taskID+"/"+sessionID)
}

type replyEnv struct {
	*conversationTestDeps
	messenger *fakeReplyMessenger
	notifier  *fakeReplyNotifier
}

func newReplyEnv(t *testing.T) *replyEnv {
	t.Helper()
	deps := newConversationTestDeps(t)
	deps.svc.phase2, deps.svc.phase3 = true, true
	env := &replyEnv{conversationTestDeps: deps, messenger: &fakeReplyMessenger{}, notifier: &fakeReplyNotifier{}}
	deps.svc.SetReplyDeps(env.messenger, env.notifier)
	return env
}

func (e *replyEnv) propose(t *testing.T) *Proposal {
	t.Helper()
	p := &Proposal{CoordinatorID: e.coordinator.ID, WorkspaceID: e.coordinator.WorkspaceID, Spec: sampleSpec()}
	if err := e.svc.store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *replyEnv) reply(p *Proposal, text string) (*Proposal, error) {
	return e.svc.ReplyToProposal(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID, ReplyRequest{Text: text})
}

func replyCount(label string) int64 {
	if v, ok := replyTotal.Get(label).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

func TestReplyDeliversOnceToTheConversation(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	before := replyCount("true")

	got, err := env.reply(p, "  Split it in two.  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ProposalStatusReturned || got.ReplyText == nil || *got.ReplyText != "Split it in two." {
		t.Fatalf("proposal = %+v", got)
	}
	if got.ReplyDeliveredAt == nil || got.ReplyDeliveryClaimedAt != nil {
		t.Fatalf("delivered=%v claimed=%v", got.ReplyDeliveredAt, got.ReplyDeliveryClaimedAt)
	}
	key := "coordinator-reply:" + p.ID
	if env.messenger.calls != 1 || env.messenger.ids[0] != key || env.messenger.queued[0].ID != key {
		t.Fatalf("messenger = %+v", env.messenger)
	}
	want := `Reply to your proposal "Do the thing" (proposal ` + p.ID + `): Split it in two.. ` +
		`If you still think the work is needed, propose it again with in_reply_to set to ` + p.ID + `.`
	if env.messenger.reqs[0].Content != want {
		t.Fatalf("content = %q, want %q", env.messenger.reqs[0].Content, want)
	}
	qm := env.messenger.queued[0].Metadata
	if qm["user_message_recorded"] != true || qm[messagequeue.MetadataDurableTranscriptMessageID] != key ||
		qm[orchestrator.MetaKeyTurnStartAlreadyProcessed] != true {
		t.Fatalf("queue metadata = %+v", qm)
	}
	if env.messenger.reqs[0].Metadata[MetaKeyReplyProposalID] != p.ID {
		t.Fatalf("message metadata = %+v", env.messenger.reqs[0].Metadata)
	}
	if len(env.notifier.calls) != 1 {
		t.Fatalf("notify calls = %v", env.notifier.calls)
	}
	if replyCount("true") != before+1 {
		t.Fatalf("delivered counter did not advance")
	}
	page, err := env.svc.ListActivity(context.Background(), p.WorkspaceID, p.CoordinatorID, ListActivityParams{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range page.Rows {
		if row.Outcome == ActivityReturned && row.Detail == "Split it in two." {
			found = true
		}
	}
	if !found {
		t.Fatalf("no returned activity row in %+v", page.Rows)
	}
}

func TestReplyFailedDeliveryStaysReturnedAndRedeliversOnce(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	env.messenger.err = errors.New("queue full")
	before := replyCount("false")

	got, err := env.reply(p, "later")
	if err != nil {
		t.Fatalf("a failed delivery must still answer 200: %v", err)
	}
	if got.Status != ProposalStatusReturned || got.ReplyDeliveredAt != nil || got.ReplyDeliveryClaimedAt != nil {
		t.Fatalf("proposal = %+v", got)
	}
	if replyCount("false") != before+1 {
		t.Fatal("undelivered counter did not advance")
	}

	env.messenger.err = nil
	again, err := env.svc.DeliverReply(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID)
	if err != nil || again.ReplyDeliveredAt == nil {
		t.Fatalf("deliver = %+v, %v", again, err)
	}
	calls := env.messenger.calls
	if _, err := env.svc.DeliverReply(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID); err != nil {
		t.Fatal(err)
	}
	if env.messenger.calls != calls {
		t.Fatal("a delivered reply must not be sent again")
	}
}

func TestReplyDeliverReplayStillNotifies(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	if _, err := env.reply(p, "x"); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after the message committed but before it was recorded delivered.
	if _, err := env.svc.store.db.Exec(env.svc.store.db.Rebind(
		`UPDATE coordinator_proposals SET reply_delivered_at = NULL WHERE id = ?`), p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := env.svc.DeliverReply(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID)
	if err != nil || got.ReplyDeliveredAt == nil {
		t.Fatalf("deliver = %+v, %v", got, err)
	}
	if len(env.messenger.created) != 2 || env.messenger.created[1] || len(env.notifier.calls) != 2 {
		t.Fatalf("created=%v notifies=%v", env.messenger.created, env.notifier.calls)
	}
}

func TestReplyValidation(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	for _, text := range []string{"", "   ", strings.Repeat("a", 2001)} {
		_, err := env.reply(p, text)
		assertFieldError(t, err, "text")
	}
	if _, err := env.reply(p, strings.Repeat("é", 2000)); err != nil {
		t.Fatalf("2000 code points must be accepted: %v", err)
	}

	other := env.propose(t)
	if _, err := env.svc.store.db.Exec(env.svc.store.db.Rebind(
		`UPDATE coordinator_proposals SET kind = 'improvement' WHERE id = ?`), other.ID); err != nil {
		t.Fatal(err)
	}
	_, err := env.reply(other, "")
	assertFieldError(t, err, "kind")
}

func TestReplyConflictsAndPhaseGate(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	if _, err := env.reply(p, "once"); err != nil {
		t.Fatal(err)
	}
	var conflict *ProposalConflictError
	if _, err := env.reply(p, "twice"); !errors.As(err, &conflict) || conflict.Proposal.Status != ProposalStatusReturned {
		t.Fatalf("second reply err = %v", err)
	}
	if _, err := env.svc.ApproveProposal(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID, ApproveProposalRequest{}); !errors.As(err, &conflict) {
		t.Fatalf("approve err = %v", err)
	}
	if _, err := env.svc.RejectProposal(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID, RejectProposalRequest{}); !errors.As(err, &conflict) {
		t.Fatalf("reject err = %v", err)
	}
	if _, err := env.svc.DeliverReply(context.Background(), p.WorkspaceID, p.CoordinatorID, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deliver missing err = %v", err)
	}
	pending := env.propose(t)
	if _, err := env.svc.DeliverReply(context.Background(), pending.WorkspaceID, pending.CoordinatorID, pending.ID); !errors.As(err, &conflict) {
		t.Fatalf("deliver pending err = %v", err)
	}
	env.svc.phase3 = false
	if _, err := env.reply(pending, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("phase 3 off err = %v", err)
	}
}

func TestReplyRaceSendsExactlyOnce(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflicts := 0, 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := env.reply(p, "race")
			var conflict *ProposalConflictError
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.As(err, &conflict):
				conflicts++
			default:
				t.Errorf("err = %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || conflicts != n-1 || env.messenger.calls != 1 {
		t.Fatalf("ok=%d conflicts=%d sends=%d", ok, conflicts, env.messenger.calls)
	}
}

func TestReplyDeliverRaceSendsOnceRecorded(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	env.messenger.err = errors.New("down")
	if _, err := env.reply(p, "x"); err != nil {
		t.Fatal(err)
	}
	env.messenger.err = nil
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := env.svc.DeliverReply(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID); err != nil {
				t.Errorf("deliver: %v", err)
			}
		}()
	}
	wg.Wait()
	got, err := env.svc.store.GetProposal(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID, true)
	if err != nil || got.ReplyDeliveredAt == nil {
		t.Fatalf("proposal = %+v, %v", got, err)
	}
	for _, c := range env.messenger.created[1:] {
		if c {
			t.Fatal("more than one message was created")
		}
	}
}

func TestProposeInReplyToRules(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	c := env.coordinator
	if err := env.svc.checkInReplyTo(context.Background(), c, p.ID); err == nil {
		t.Fatal("a pending proposal must be refused")
	}
	if _, err := env.reply(p, "revise"); err != nil {
		t.Fatal(err)
	}
	if err := env.svc.checkInReplyTo(context.Background(), c, p.ID); err != nil {
		t.Fatalf("returned proposal refused: %v", err)
	}
	if err := env.svc.checkInReplyTo(context.Background(), c, p.ID); err != nil {
		t.Fatalf("several proposals may name one returned row: %v", err)
	}
	assertFieldError(t, env.svc.checkInReplyTo(context.Background(), c, "missing"), "in_reply_to")
	env.svc.phase3 = false
	assertFieldError(t, env.svc.checkInReplyTo(context.Background(), c, p.ID), "in_reply_to")
	if err := env.svc.checkInReplyTo(context.Background(), c, ""); err != nil {
		t.Fatal(err)
	}
}
