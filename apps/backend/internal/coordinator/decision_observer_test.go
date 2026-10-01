package coordinator

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/outcomes"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

type recordingObserver struct {
	mu     sync.Mutex
	events []DecisionEvent
	panics bool
}

func (r *recordingObserver) OnDecision(_ context.Context, e DecisionEvent) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
	if r.panics {
		panic("observer failure")
	}
}

func (r *recordingObserver) only(t *testing.T) DecisionEvent {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) != 1 {
		t.Fatalf("events = %+v, want exactly one", r.events)
	}
	return r.events[0]
}

func TestDecisionObserver_ApproveNotifiesOnce(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	if _, err := svc.ApproveProposal(authedContext("u1"), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	e := obs.only(t)
	if e.ProposalID != p.ID || e.Decision != outcomes.DecisionApproved || e.Automatic || e.ActorUserID != "u1" || len(e.EditedFields) != 0 {
		t.Fatalf("event = %+v", e)
	}
}

func TestDecisionObserver_ApproveWithEditsNamesFieldsOnly(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{"title": json.RawMessage(`"Another title"`)}); err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	e := obs.only(t)
	if e.Decision != outcomes.DecisionEdited || !reflect.DeepEqual(e.EditedFields, []string{"title"}) {
		t.Fatalf("event = %+v", e)
	}
}

func TestDecisionObserver_RejectNotifiesWithCodeNotText(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	p := insertProposal(t, store, c, sampleSpec())

	if _, err := svc.RejectProposal(authedContext("u1"), "ws-1", c.ID, p.ID, RejectProposalRequest{Reason: strPtr("secret words")}); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	e := obs.only(t)
	if e.Decision != outcomes.DecisionRejected || e.ReasonCode != "other" || e.ActorUserID != "u1" {
		t.Fatalf("event = %+v", e)
	}
}

func TestDecisionObserver_ConflictNotifiesNothing(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	p := insertProposal(t, store, c, sampleSpec())
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); err == nil {
		t.Fatal("second reject succeeded")
	}
	obs.only(t)
}

func TestDecisionObserver_PanicNeverFailsTheDecision(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	svc.SetDecisionObserver(&recordingObserver{panics: true})
	p := insertProposal(t, store, c, sampleSpec())
	got, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{})
	if err != nil || got.Status != ProposalStatusRejected {
		t.Fatalf("RejectProposal = %+v, %v", got, err)
	}
}

func TestDecisionObserver_NilObserverIsNoOp(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
}

func TestDecisionObserver_ReturnNotifiesReturned(t *testing.T) {
	env := newReplyEnv(t)
	obs := &recordingObserver{}
	env.svc.SetDecisionObserver(obs)
	p := env.propose(t)
	if _, err := env.reply(p, "Split it in two."); err != nil {
		t.Fatal(err)
	}
	e := obs.only(t)
	if e.Decision != outcomes.DecisionReturned || e.ProposalID != p.ID || e.Automatic {
		t.Fatalf("event = %+v", e)
	}
}

func TestDecisionObserver_UndoNotifiesUndoneWithTheUndoTime(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	seedApproved(t, store, c, "r1", ActionCreateTask, time.Now().UTC(), func(r *ActivityRow) {
		taskID := "t1"
		r.TargetTaskID = &taskID
		r.ProposalID = &p.ID
	})
	if _, err := svc.UndoActivity(authedContext("user-7"), "ws-1", c.ID, "r1"); err != nil {
		t.Fatal(err)
	}
	e := obs.only(t)
	if e.Decision != outcomes.DecisionUndone || e.ProposalID != p.ID || e.At.IsZero() || e.ActorUserID != "user-7" {
		t.Fatalf("event = %+v", e)
	}
}

func TestDecisionObserver_RecoveredApprovalKeepsTheDecidingUser(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	p := insertProposal(t, store, c, sampleSpec())
	if matched, err := store.ClaimProposal(context.Background(), p.ID, "old-tok", sampleSpec(), "user-9", time.Now().Add(-10*time.Minute)); err != nil || !matched {
		t.Fatalf("ClaimProposal matched=%v err=%v", matched, err)
	}
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	svc.StartupRecoveryPass(context.Background(), time.Now())

	if e := obs.only(t); e.Decision != outcomes.DecisionApproved || e.ActorUserID != "user-9" {
		t.Fatalf("event = %+v, want the claiming user", e)
	}
}

func TestDecisionObserver_FailedApprovalNotifiesNothing(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-10*time.Minute))
	svc.SetDecisionDeps(tasks, &fakeStepReader{steps: []*workflowmodels.WorkflowStep{
		{ID: "step-1", IsStartStep: true, Events: workflowmodels.StepEvents{
			OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}},
		}},
	}}, nil)

	svc.StartupRecoveryPass(context.Background(), time.Now())

	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID, false)
	if err != nil || got.Status != ProposalStatusFailed {
		t.Fatalf("proposal = %+v err = %v, want failed", got, err)
	}
	obs.mu.Lock()
	defer obs.mu.Unlock()
	if len(obs.events) != 0 {
		t.Fatalf("events = %+v, want none", obs.events)
	}
}
