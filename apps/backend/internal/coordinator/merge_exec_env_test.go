package coordinator

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type mergeCall struct {
	Key    mergestate.ChangeKey
	Head   string
	Method string
}

// fakeMerger is a programmable provider seam.
type fakeMerger struct {
	mu         sync.Mutex
	snap       mergestate.Snapshot
	snapErr    error
	methods    MethodSet
	methodsErr error
	result     MergeResult
	mergeErr   error
	calls      []mergeCall
	reads      int
	onMerge    func()
	onRead     func(n int)
	pages      [][]string
	pagesErr   error
	pageCalls  int
}

func (f *fakeMerger) ReadLive(context.Context, string, mergestate.ChangeKey) (mergestate.Snapshot, error) {
	f.mu.Lock()
	f.reads++
	n, hook, snap, err := f.reads, f.onRead, f.snap, f.snapErr
	f.mu.Unlock()
	if hook != nil {
		hook(n)
	}
	return snap, err
}

func (f *fakeMerger) Methods(context.Context, string, mergestate.ChangeKey) (MethodSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.methods, f.methodsErr
}

func (f *fakeMerger) Merge(_ context.Context, _ string, key mergestate.ChangeKey, head, method string) (MergeResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, mergeCall{Key: key, Head: head, Method: method})
	hook, res, err := f.onMerge, f.result, f.mergeErr
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return res, err
}

func (f *fakeMerger) CommitMessages(_ context.Context, _ string, _ mergestate.ChangeKey, _ string, page int) ([]string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pageCalls++
	if f.pagesErr != nil || page > len(f.pages) {
		return nil, false, f.pagesErr
	}
	return f.pages[page-1], page < len(f.pages), nil
}

func (f *fakeMerger) mergeCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeMerger) set(fn func(*fakeMerger)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func greenSnapshot(head string) mergestate.Snapshot {
	return mergestate.Snapshot{
		Key: testKey, State: mergestate.StateOpen, HeadSHA: head, TargetBranch: "main", Draft: mergestate.Met,
		Pipeline: mergestate.PipelineGreen, Mergeable: mergestate.MergeableReading{Mergeable: mergestate.Met},
		Conflict: mergestate.ConflictClear, Approvals: mergestate.Met, UnresolvedThreads: 0,
	}
}

type mergeExec struct {
	*wakeEnv
	merger *fakeMerger
	ctx    context.Context
}

// newMergeExec builds a coordinator that watches task tm, linked to change
// testChangeKey of repository r1, whose policy is requires_approval.
func newMergeExec(t *testing.T) *mergeExec {
	t.Helper()
	e, _ := newObserveEnv(t)
	mustExec(t, e.store, fmt.Sprintf(`CREATE TABLE repositories (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL, deleted_at %s);
		INSERT INTO repositories (id, workspace_id, name) VALUES ('r1','ws-1','alpha');`, tsType(e.store)))
	e.svc.SetPrincipalValidator(fakePrincipals{ok: map[string]bool{"mgr": true}})
	m := &fakeMerger{snap: greenSnapshot("h1"), methods: MethodSet{Allowed: []string{"squash"}, Default: "squash"},
		result: MergeResult{Outcome: MergeAccepted, MergeCommit: "mc1"}}
	e.svc.SetChangeMerger(m)
	env := &mergeExec{wakeEnv: e, merger: m, ctx: managerCtx("mgr")}
	env.setPolicy(t, RepoRequiresApproval, 0)
	return env
}

func (e *mergeExec) setPolicy(t *testing.T, policy RepoPolicy, since int) {
	t.Helper()
	err := e.store.withCoordinatorLock(e.ctx, e.fix.c.ID, func(tx coordinatorExec) error {
		return e.store.writeRepoPolicyPlan(e.ctx, tx, e.fix.c.ID, "", policyPlan{Rows: []RepoPolicyRow{
			{RepositoryID: "r1", Policy: policy, SelectedBy: "mgr", AutomaticSinceRevision: since}}})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (e *mergeExec) propose(t *testing.T, head string) *MergeProposeResult {
	t.Helper()
	res, err := e.svc.ProposeMerge(e.ctx, e.fix.c.ID, []byte(fmt.Sprintf(`{"task_id":"tm","expected_head_sha":%q}`, head)))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (e *mergeExec) approve(id string) (*Proposal, error) {
	return e.svc.ApproveProposal(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id, ApproveProposalRequest{})
}

func (e *mergeExec) status(t *testing.T, id string) (string, string) {
	t.Helper()
	p, err := e.store.GetProposalAnyKind(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	code := ""
	if p.Error != nil {
		code = *p.Error
	}
	return string(p.Status), code
}

func (e *mergeExec) activity(t *testing.T, outcome ActivityOutcome, reason string) int {
	t.Helper()
	var n int
	err := e.store.db.QueryRow(e.store.db.Rebind(`SELECT COUNT(*) FROM coordinator_activity WHERE action_class = 'merge' AND outcome = ? AND reason_code = ?`),
		string(outcome), reason).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// admit makes automatic admission pass: autonomy on, contained, measurable
// spend under the ceiling and a live conversation task.
func (e *mergeExec) admit(t *testing.T) {
	t.Helper()
	ledger := &fakeSpendLedger{}
	ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{CostSubcents: 10}, nil }
	conv := newFakeConversationTasks()
	conv.tasks[ceilingConvTask] = spendConvTask(ceilingConvTask, e.fix.c.ID, false)
	e.svc.conversationTasks = conv
	e.svc.SetSpendDeps(ledger, &fakeActiveTurns{turns: map[string]string{}}, &fakeCanceller{})
	e.svc.SetContainment(newChecker(contained()))
	mustExec(t, e.store, `UPDATE coordinators SET autonomy_enabled = ?, conversation_task_id = ?, cost_ceiling_subcents = ? WHERE id = ?`,
		true, ceilingConvTask, 1000, e.fix.c.ID)
}
