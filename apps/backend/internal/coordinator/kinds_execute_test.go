package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type fakeKindTasks struct {
	target *TargetTask
	err    error
	live   bool
}

func (f *fakeKindTasks) GetTarget(context.Context, string) (*TargetTask, error) {
	return f.target, f.err
}
func (f *fakeKindTasks) HasLiveExecution(context.Context, string) bool { return f.live }

type fakeResumer struct {
	started bool
	err     error
	calls   int
	release chan struct{}
	done    chan struct{}
}

func (f *fakeResumer) ResumeTaskSession(context.Context, string, string) (bool, error) {
	f.calls++
	if f.release != nil {
		<-f.release
	}
	if f.done != nil {
		defer close(f.done)
	}
	return f.started, f.err
}

type fakeMessenger struct {
	mu      sync.Mutex
	prompts []string
	err     error
}

func (f *fakeMessenger) DeliverQueued(_ context.Context, _, sessionID, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, prompt)
	return sessionID, f.err
}

func (f *kindsFixture) insertKind(t *testing.T, kind, spec string) *Proposal {
	t.Helper()
	f.seq++
	target := fmt.Sprintf("task-%d", f.seq)
	var body map[string]json.RawMessage
	if json.Unmarshal([]byte(spec), &body) == nil {
		if _, ok := body["task_id"]; ok {
			body["task_id"], _ = json.Marshal(target)
			raw, _ := json.Marshal(body)
			spec = string(raw)
		}
	}
	p := &Proposal{CoordinatorID: f.c.ID, WorkspaceID: f.c.WorkspaceID, Kind: kind, TargetTaskID: &target, RawSpec: spec}
	if err := f.store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *kindsFixture) approve(p *Proposal, edits ApproveProposalRequest) (*Proposal, error) {
	return f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, edits)
}

func idleSession() *TargetTask {
	return &TargetTask{ID: "task-0", WorkspaceID: "ws-1", Primary: &TargetSession{ID: "s1", State: "IDLE", HasExecutorRecord: true}}
}

func TestMoveExecute_Classification(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *kindsFixture)
		want  string
	}{
		{"archived", func(f *kindsFixture) { now := time.Now(); f.undo.tasks["task-0"].ArchivedAt = &now }, failTaskArchived},
		{"deleted", func(f *kindsFixture) { f.undo.taskErr = ErrTaskNotFound }, failTaskArchived},
		{"left workflow", func(f *kindsFixture) { f.undo.tasks["task-0"].WorkflowID = "wf-2" }, failTaskLeftWF},
		{"step missing", func(f *kindsFixture) { delete(f.undo.steps, "manual-step") }, failStepMissing},
		{"done step", func(f *kindsFixture) { f.undo.steps["manual-step"].CompletesOnEnter = true }, failStepIsDone},
		{"starts agent", func(f *kindsFixture) { f.undo.nodes = []StepNode{{ID: "manual-step", AutoStartOnEnter: true}} }, failStepStartsAgent},
		{"running session", func(f *kindsFixture) { f.undo.sessions = true }, failAgentRunning},
		{"step full", func(f *kindsFixture) { f.undo.moveErr = ErrWIPLimitExceeded }, failStepFull},
		{"moved", func(f *kindsFixture) { f.undo.moveErr = ErrMoveConflict }, failMoved},
		{"other error", func(f *kindsFixture) { f.undo.moveErr = errors.New("boom") }, "boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newKindsFixture(t)
			tc.setup(f)
			p := f.insertMove(t)
			got, err := f.approve(p, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != ProposalStatusFailed || f.errorOf(t, p.ID) != tc.want {
				t.Fatalf("status=%q error=%q, want failed %q", got.Status, f.errorOf(t, p.ID), tc.want)
			}
		})
	}
}

func TestMoveExecute_NoopAndQueued(t *testing.T) {
	f := newKindsFixture(t)
	f.undo.tasks["task-0"].WorkflowStepID = "manual-step"
	f.undo.steps["manual-step"].CompletesOnEnter = true
	p := f.insertMove(t)
	got, err := f.approve(p, nil)
	if err != nil || got.Status != ProposalStatusApproved || len(f.undo.moves) != 0 {
		t.Fatalf("got=%+v err=%v moves=%d, want approved noop with no move", got, err, len(f.undo.moves))
	}
	if got.OutcomeJSON == nil || *got.OutcomeJSON != `{"from_step_id":"manual-step","noop":true,"to_step_id":"manual-step"}` {
		t.Fatalf("outcome = %v", got.OutcomeJSON)
	}

	q := newKindsFixture(t)
	q.undo.admitted = false
	qp := q.insertMove(t)
	got, err = q.approve(qp, nil)
	if err != nil || got.Status != ProposalStatusApproved || got.OutcomeJSON == nil || *got.OutcomeJSON != `{"from_step_id":"step-1","queued":true,"to_step_id":"manual-step"}` {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if q.undo.moves[0].Opts.ExpectedWorkflowID != "wf-1" {
		t.Fatalf("move opts = %+v, want ExpectedWorkflowID wf-1", q.undo.moves[0].Opts)
	}
}

func TestMoveExecute_RecordsFromStepBeforeMove(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	var during *string
	f.undo.onMove = func() {
		got, _ := f.store.GetProposal(context.Background(), "ws-1", f.c.ID, p.ID, true)
		during = got.OutcomeJSON
	}
	if _, err := f.approve(p, nil); err != nil {
		t.Fatal(err)
	}
	if during == nil || *during != `{"from_step_id":"step-1","to_step_id":"manual-step"}` {
		t.Fatalf("outcome during move = %v", during)
	}
}

func TestMoveExecute_FromStepWriteFencedNoMove(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	fenced, _ := f.watchFenced(t)
	f.undo.onGetStep = func() { f.svc.runApprovalSweepPass(context.Background(), time.Now().Add(time.Hour)) }
	got, err := f.approve(p, nil)
	if err != nil || got.Status != ProposalStatusFailed || len(f.undo.moves) != 0 || f.errorOf(t, p.ID) != "outcome_unknown" {
		t.Fatalf("got=%+v err=%v moves=%d error=%q, want failed outcome_unknown and no move call", got, err, len(f.undo.moves), f.errorOf(t, p.ID))
	}
	if fenced.Load() != 1 {
		t.Fatalf("execute_settle_fenced warnings = %d, want 1", fenced.Load())
	}
}

func TestExecuteDeadline(t *testing.T) {
	cases := []struct {
		name       string
		hold       func(ctx context.Context) error
		wantStatus ProposalStatus
		wantErr    string
	}{
		{"nil error after the deadline", func(ctx context.Context) error { <-ctx.Done(); return nil }, ProposalStatusApproved, ""},
		{"deadline error", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, ProposalStatusFailed, "outcome_unknown"},
		{"other error after the deadline", func(ctx context.Context) error { <-ctx.Done(); return errors.New("connection reset") }, ProposalStatusFailed, "outcome_unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newKindsFixture(t)
			f.svc.executeTimeout = 20 * time.Millisecond
			f.undo.onMoveCtx = tc.hold
			p := f.insertMove(t)
			got, err := f.approve(p, nil)
			if err != nil || got.Status != tc.wantStatus {
				t.Fatalf("got=%+v err=%v, want %s", got, err, tc.wantStatus)
			}
			if tc.wantErr != "" && f.errorOf(t, p.ID) != tc.wantErr {
				t.Fatalf("error = %q, want %q", f.errorOf(t, p.ID), tc.wantErr)
			}
		})
	}
}

func TestResumeExecute(t *testing.T) {
	f := newKindsFixture(t)
	res := &fakeResumer{started: true}
	f.svc.SetKindDeps(KindDeps{Tasks: &fakeKindTasks{target: idleSession()}, Resumer: res})
	got, err := f.approve(f.insertKind(t, ProposalKindResume, `{"task_id":"task-0"}`), nil)
	if err != nil || got.Status != ProposalStatusApproved || *got.OutcomeJSON != `{"session_id":"s1"}` {
		t.Fatalf("started: got=%+v err=%v", got, err)
	}
	res.started = false
	got, _ = f.approve(f.insertKind(t, ProposalKindResume, `{"task_id":"task-0"}`), nil)
	if got.Status != ProposalStatusApproved || *got.OutcomeJSON != `{"deferred":true,"session_id":"s1"}` {
		t.Fatalf("deferred: got=%+v", got)
	}
	res.err = errors.New("resume attempt cancelled")
	got, _ = f.approve(f.insertKind(t, ProposalKindResume, `{"task_id":"task-0"}`), nil)
	if got.Status != ProposalStatusFailed || *got.Error != "resume attempt cancelled" {
		t.Fatalf("error: got=%+v", got)
	}
	f.svc.kindDeps.Tasks = &fakeKindTasks{target: idleSession(), live: true}
	got, _ = f.approve(f.insertKind(t, ProposalKindResume, `{"task_id":"task-0"}`), nil)
	if got.Status != ProposalStatusFailed || *got.Error != failNotResumable {
		t.Fatalf("live: got=%+v", got)
	}
}

func TestResumeExecute_ReturnsAtDeadlineAndLateResultWritesNothing(t *testing.T) {
	f := newKindsFixture(t)
	fenced, warned := f.watchFenced(t)
	f.svc.executeTimeout = 20 * time.Millisecond
	res := &fakeResumer{started: true, release: make(chan struct{})}
	f.svc.SetKindDeps(KindDeps{Tasks: &fakeKindTasks{target: idleSession()}, Resumer: res})
	p := f.insertKind(t, ProposalKindResume, `{"task_id":"task-0"}`)
	got, err := f.approve(p, nil)
	if err != nil || got.Status != ProposalStatusFailed || *got.Error != "outcome_unknown" {
		t.Fatalf("got=%+v err=%v, want failed outcome_unknown", got, err)
	}
	before := f.outcomeOf(t, p.ID)
	close(res.release)
	select {
	case <-warned:
	case <-time.After(10 * time.Second):
		t.Fatal("the abandoned launch never reported its fenced result")
	}
	if fenced.Load() != 1 || statusOf(t, f.store, p.ID) != string(ProposalStatusFailed) || f.errorOf(t, p.ID) != "outcome_unknown" || f.outcomeOf(t, p.ID) != before {
		t.Fatalf("late launch result changed a settled row: warnings=%d status=%q error=%q outcome=%q", fenced.Load(), statusOf(t, f.store, p.ID), f.errorOf(t, p.ID), f.outcomeOf(t, p.ID))
	}
}

func TestMessageExecute(t *testing.T) {
	f := newKindsFixture(t)
	msg := &fakeMessenger{}
	f.svc.SetKindDeps(KindDeps{Tasks: &fakeKindTasks{target: idleSession()}, Messenger: msg})
	p := f.insertKind(t, ProposalKindMessage, `{"task_id":"task-0","text":"hello"}`)
	got, err := f.approve(p, ApproveProposalRequest{"text": []byte(`"  edited  "`)})
	if err != nil || got.Status != ProposalStatusApproved || len(msg.prompts) != 1 {
		t.Fatalf("got=%+v err=%v prompts=%d", got, err, len(msg.prompts))
	}
	if want := "\n\nedited"; len(msg.prompts[0]) < len(want) || msg.prompts[0][len(msg.prompts[0])-len(want):] != want {
		t.Fatalf("prompt = %q, want it to end with the edited text", msg.prompts[0])
	}

	msg.err = ErrMessageQueueFull
	got, _ = f.approve(f.insertKind(t, ProposalKindMessage, `{"task_id":"task-0","text":"x"}`), nil)
	if got.Status != ProposalStatusFailed || *got.Error != failQueueFull {
		t.Fatalf("queue full: got=%+v", got)
	}
	for _, state := range []string{"CREATED", "FAILED", "CANCELLED"} {
		tt := idleSession()
		tt.Primary.State = state
		f.svc.kindDeps.Tasks = &fakeKindTasks{target: tt}
		got, _ = f.approve(f.insertKind(t, ProposalKindMessage, `{"task_id":"task-0","text":"x"}`), nil)
		if got.Status != ProposalStatusFailed || *got.Error != failNotAccepting {
			t.Fatalf("%s: got=%+v", state, got)
		}
	}
}

func TestApproveEdits_NotEditableAndTextRules(t *testing.T) {
	f := newKindsFixture(t)
	f.svc.SetKindDeps(KindDeps{Tasks: &fakeKindTasks{target: idleSession()}, Messenger: &fakeMessenger{}, Resumer: &fakeResumer{}})
	for _, kind := range []string{ProposalKindResume, ProposalKindMove} {
		p := f.insertKind(t, kind, `{"task_id":"task-0"}`)
		for _, name := range editFieldNames {
			_, err := f.approve(p, ApproveProposalRequest{name: []byte(`null`)})
			var fe *FieldError
			if !errors.As(err, &fe) || fe.Message != "not_editable" {
				t.Fatalf("%s %s: err = %v, want not_editable", kind, name, err)
			}
		}
		if statusOf(t, f.store, p.ID) != string(ProposalStatusPending) {
			t.Fatalf("%s: a refused edit must not claim", kind)
		}
		_, _ = f.svc.RejectProposal(context.Background(), "ws-1", f.c.ID, p.ID, RejectProposalRequest{})
	}
	p := f.insertKind(t, ProposalKindMessage, `{"task_id":"task-0","text":"ok"}`)
	for _, body := range []string{`null`, `""`, `"   "`, `7`} {
		_, err := f.approve(p, ApproveProposalRequest{"text": []byte(body)})
		var fe *FieldError
		if !errors.As(err, &fe) || fe.Field != "text" {
			t.Fatalf("text %s: err = %v, want a text FieldError", body, err)
		}
	}
	_, err := f.approve(p, ApproveProposalRequest{"title": []byte(`"x"`)})
	var fe *FieldError
	if !errors.As(err, &fe) || fe.Field != "title" || fe.Message != "not_editable" {
		t.Fatalf("title on message: err = %v", err)
	}
}

func TestUnknownKindStaleRowIsLeftAlone(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertKind(t, "wave", `{"task_id":"task-0"}`)
	f.forceApproving(t, p, time.Now().Add(-time.Hour))
	f.svc.StartupRecoveryPass(context.Background(), time.Now())
	if got := statusOf(t, f.store, p.ID); got != string(ProposalStatusApproving) {
		t.Fatalf("status = %q, want approving (untouched)", got)
	}
}
