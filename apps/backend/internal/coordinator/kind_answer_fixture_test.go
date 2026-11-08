package coordinator

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification"
)

type fakeAnswerTargets struct {
	mu     sync.Mutex
	set    TargetSet
	err    error
	calls  int
	onRead func()
}

func (f *fakeAnswerTargets) CurrentTargets(context.Context, string, string) (TargetSet, error) {
	f.mu.Lock()
	f.calls++
	hook, set, err := f.onRead, f.set, f.err
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return set, err
}

func (f *fakeAnswerTargets) setSet(s TargetSet) {
	f.mu.Lock()
	f.set = s
	f.mu.Unlock()
}

type fakeAnswerDelivery struct {
	mu          sync.Mutex
	won         bool
	resolveErr  error
	permErr     error
	outcomes    []clarification.Outcome
	pendingIDs  []string
	permCalls   [][]string
	resolveHook func()
}

func (f *fakeAnswerDelivery) ResolveBundle(_ context.Context, pendingID string, o clarification.Outcome) (*clarification.Resolution, bool, error) {
	f.mu.Lock()
	f.outcomes = append(f.outcomes, o)
	f.pendingIDs = append(f.pendingIDs, pendingID)
	hook, won, err := f.resolveHook, f.won, f.resolveErr
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	if err != nil {
		return nil, won, err
	}
	return &clarification.Resolution{PendingID: pendingID}, won, nil
}

func (f *fakeAnswerDelivery) RespondToPermission(_ context.Context, taskID, sessionID, requestID, pendingID, optionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.permCalls = append(f.permCalls, []string{taskID, sessionID, requestID, pendingID, optionID})
	return f.permErr
}

func (f *fakeAnswerDelivery) resolveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.outcomes) + len(f.permCalls)
}

type answerFx struct {
	*kindsFixture
	targets  *fakeAnswerTargets
	delivery *fakeAnswerDelivery
}

func yesNo(id string) clarification.Question {
	return clarification.Question{ID: id, Title: id, Prompt: "Prompt " + id,
		Options: []clarification.Option{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}}}
}

func clarificationSet(pendingID string, qs ...clarification.Question) TargetSet {
	return TargetSet{Clarification: &ClarificationTarget{PendingID: pendingID, SessionID: "s1", Questions: qs}}
}

func permissionSet(pendingID, requestID string) TargetSet {
	return TargetSet{Permission: &PermissionTarget{PendingID: pendingID, RequestID: requestID, SessionID: "s1", Title: "Run ls",
		Summary: `{"action_type":"execute"}`, Options: []PermissionOptionTarget{
			{ID: "allow", Name: "Allow", Kind: "allow_once"}, {ID: "deny", Name: "Deny", Kind: "reject_once"},
			{ID: "always", Name: "Always", Kind: "allow_always"}}}}
}

func newAnswerFx(t *testing.T) *answerFx {
	t.Helper()
	f := proposeFixture(t)
	f.svc.answer = true
	fx := &answerFx{kindsFixture: f, targets: &fakeAnswerTargets{set: clarificationSet("p1", yesNo("q1"), yesNo("q2"))},
		delivery: &fakeAnswerDelivery{won: true}}
	f.svc.answerTargets = fx.targets
	f.svc.SetKindDeps(KindDeps{Tasks: f.tasks, Messenger: &fakeMessenger{}, Resumer: &fakeResumer{}, Answer: fx.delivery})
	return fx
}

const clarificationArgs = `{"task_id":"task-0","target":"clarification","pending_id":"p1","confidence":"high","needs_decision":false,
"draft":{"answers":[{"question_id":"q1","option_ids":["a"]},{"question_id":"q2","option_ids":["b"],"custom_text":"  why  "}]}}`

const permissionArgs = `{"task_id":"task-0","target":"permission","pending_id":"pp1","confidence":"low","needs_decision":true,
"draft":{"option_id":"allow"},"sources":[{"ref":"docs/x.md:3","label":"doc"}],"rationale":"safe"}`

func (fx *answerFx) proposeAnswer(args string, ids ...string) (*Proposal, bool, error) {
	return fx.propose(ProposalKindAnswer, args, ids...)
}

func (fx *answerFx) mustPropose(t *testing.T, args string) *Proposal {
	t.Helper()
	p, _, err := fx.proposeAnswer(args)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return p
}

func (fx *answerFx) reload(t *testing.T, id string) *Proposal {
	t.Helper()
	p, err := fx.store.GetProposalAnyKind(context.Background(), fx.c.WorkspaceID, fx.c.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (fx *answerFx) ageProposal(t *testing.T, id string) {
	t.Helper()
	if _, err := fx.store.db.Exec(fx.store.db.Rebind(`UPDATE coordinator_proposals SET created_at = ? WHERE id = ?`),
		time.Now().Add(-time.Hour).UTC(), id); err != nil {
		t.Fatal(err)
	}
}

func specOf(t *testing.T, p *Proposal) answerSpec {
	t.Helper()
	var s answerSpec
	if err := json.Unmarshal([]byte(p.RawSpec), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func openAnswerCount(t *testing.T, fx *answerFx) int {
	t.Helper()
	list, err := fx.store.ListProposals(context.Background(), fx.c.WorkspaceID, fx.c.ID, ListProposalsAll, true)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, p := range list {
		if p.Kind == ProposalKindAnswer && (p.Status == ProposalStatusPending || p.Status == ProposalStatusApproving || p.Status == ProposalStatusFailed) {
			n++
		}
	}
	return n
}
