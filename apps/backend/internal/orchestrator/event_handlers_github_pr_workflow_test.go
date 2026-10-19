package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	prWorkflowTaskID    = "pr-task"
	prWorkflowSessionID = "pr-session"
)

type prWorkflowFixture struct {
	svc         *Service
	repo        *sqliterepo.Repository
	github      *mockGitHubService
	recorder    *fakeStepHistoryRecorder
	onEnterDone chan struct{}
}

// newPRWorkflowFixture seeds a task on step_accept, whose on_pr_merged moves
// to step_done and whose on_pr_closed moves to step_exec.
func newPRWorkflowFixture(t *testing.T, acceptEvents wfmodels.StepEvents) *prWorkflowFixture {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, prWorkflowTaskID, prWorkflowSessionID, "step_accept")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_exec"] = &wfmodels.WorkflowStep{
		ID: "step_exec", WorkflowID: "wf1", Name: "Execution", Position: 0,
	}
	stepGetter.steps["step_accept"] = &wfmodels.WorkflowStep{
		ID: "step_accept", WorkflowID: "wf1", Name: "Acceptance", Position: 1,
		Events: acceptEvents,
	}
	stepGetter.steps["step_done"] = &wfmodels.WorkflowStep{
		ID: "step_done", WorkflowID: "wf1", Name: "Done", Position: 2,
	}

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{repoForExecutionLookup: repo})
	gh := &mockGitHubService{}
	svc.SetGitHubService(gh)
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder
	onEnterDone := make(chan struct{}, 4)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}
	return &prWorkflowFixture{svc: svc, repo: repo, github: gh, recorder: recorder, onEnterDone: onEnterDone}
}

func prMergedAcceptEvents() wfmodels.StepEvents {
	return wfmodels.StepEvents{
		OnPRMerged: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]any{"step_id": "step_done"}},
		},
		OnPRClosed: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]any{"step_id": "step_exec"}},
		},
	}
}

func linkedTaskPR(number int, state string) *github.TaskPR {
	return &github.TaskPR{
		TaskID: prWorkflowTaskID, Owner: "acme", Repo: "widgets", PRNumber: number,
		PRURL: "https://github.com/acme/widgets/pull/1", State: state,
	}
}

func (f *prWorkflowFixture) process(t *testing.T, pr *github.TaskPR) bool {
	t.Helper()
	transitioned, err := f.svc.processTaskPRWorkflowEvent(context.Background(), pr)
	if err != nil {
		t.Fatalf("processTaskPRWorkflowEvent: %v", err)
	}
	return transitioned
}

func (f *prWorkflowFixture) taskStep(t *testing.T) string {
	t.Helper()
	task, err := f.repo.GetTask(context.Background(), prWorkflowTaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	return task.WorkflowStepID
}

func (f *prWorkflowFixture) waitOnEnter(t *testing.T) {
	t.Helper()
	select {
	case <-f.onEnterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for processOnEnter goroutine")
	}
}

func TestProcessTaskPRWorkflowEvent_MergedMovesTaskOnceAndRecordsTrigger(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	merged := linkedTaskPR(1, "merged")
	f.github.taskPRs = []*github.TaskPR{merged}

	if !f.process(t, merged) {
		t.Fatal("expected merged PR to transition the task")
	}
	f.waitOnEnter(t)
	if got := f.taskStep(t); got != "step_done" {
		t.Fatalf("task step = %q, want step_done", got)
	}

	calls := f.recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("step history calls = %d, want 1", len(calls))
	}
	call := calls[0]
	if call.trigger != wfmodels.StepTransitionTriggerPRMerged {
		t.Errorf("history trigger = %q, want %q", call.trigger, wfmodels.StepTransitionTriggerPRMerged)
	}
	if call.actorID != nil {
		t.Errorf("history actor id = %q, want nil (system move)", *call.actorID)
	}
	if call.fromStepID != "step_accept" || call.toStepID != "step_done" {
		t.Errorf("history move = %s -> %s, want step_accept -> step_done", call.fromStepID, call.toStepID)
	}

	if f.process(t, merged) {
		t.Fatal("expected a repeated merged event not to transition again")
	}
	if got := len(f.recorder.Calls()); got != 1 {
		t.Fatalf("step history calls after redelivery = %d, want 1", got)
	}
}

func TestProcessTaskPRWorkflowEvent_MergedWaitsForEveryLinkedPR(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	first := linkedTaskPR(1, "merged")
	second := linkedTaskPR(2, "open")
	f.github.taskPRs = []*github.TaskPR{first, second}

	if f.process(t, first) {
		t.Fatal("expected no transition while another linked PR is open")
	}
	if got := f.taskStep(t); got != "step_accept" {
		t.Fatalf("task step = %q, want step_accept", got)
	}

	second.State = "merged"
	if !f.process(t, second) {
		t.Fatal("expected transition once every linked PR is merged")
	}
	f.waitOnEnter(t)
	if got := f.taskStep(t); got != "step_done" {
		t.Fatalf("task step = %q, want step_done", got)
	}
}

func TestProcessTaskPRWorkflowEvent_ClosedRunsOnPRClosed(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	closedAt := time.Now().UTC()
	closed := linkedTaskPR(1, "closed")
	closed.ClosedAt = &closedAt
	f.github.taskPRs = []*github.TaskPR{closed}

	if !f.process(t, closed) {
		t.Fatal("expected closed PR to run on_pr_closed")
	}
	f.waitOnEnter(t)
	if got := f.taskStep(t); got != "step_exec" {
		t.Fatalf("task step = %q, want step_exec", got)
	}
	calls := f.recorder.Calls()
	if len(calls) != 1 || calls[0].trigger != wfmodels.StepTransitionTriggerPRClosed {
		t.Fatalf("history calls = %#v, want one on_pr_closed row", calls)
	}
}

func TestProcessTaskPRWorkflowEvent_IgnoredWithoutStepEvent(t *testing.T) {
	f := newPRWorkflowFixture(t, wfmodels.StepEvents{})
	merged := linkedTaskPR(1, "merged")
	f.github.taskPRs = []*github.TaskPR{merged}

	if f.process(t, merged) {
		t.Fatal("expected a step without on_pr_merged to ignore the merge")
	}
	if got := f.taskStep(t); got != "step_accept" {
		t.Fatalf("task step = %q, want step_accept", got)
	}
	if got := len(f.recorder.Calls()); got != 0 {
		t.Fatalf("step history calls = %d, want 0", got)
	}
}

func TestProcessTaskPRWorkflowEvent_IgnoresPRNotLinkedToTask(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	f.github.taskPRs = []*github.TaskPR{linkedTaskPR(1, "open")}

	if f.process(t, linkedTaskPR(99, "merged")) {
		t.Fatal("expected a PR that is not linked to the task to be ignored")
	}
	if got := f.taskStep(t); got != "step_accept" {
		t.Fatalf("task step = %q, want step_accept", got)
	}
}

func TestProcessTaskPRWorkflowEvent_MovesTaskWithCompletedSessionWithoutReviving(t *testing.T) {
	f := newPRWorkflowFixture(t, prMergedAcceptEvents())
	ctx := context.Background()
	if err := f.repo.UpdateTaskSessionState(ctx, prWorkflowSessionID, models.TaskSessionStateCompleted, ""); err != nil {
		t.Fatalf("complete session: %v", err)
	}
	merged := linkedTaskPR(1, "merged")
	f.github.taskPRs = []*github.TaskPR{merged}

	if !f.process(t, merged) {
		t.Fatal("expected the merge to move a task whose only session is completed")
	}
	if got := f.taskStep(t); got != "step_done" {
		t.Fatalf("task step = %q, want step_done", got)
	}
	session, err := f.repo.GetTaskSession(ctx, prWorkflowSessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.State != models.TaskSessionStateCompleted {
		t.Fatalf("session state = %q, want COMPLETED (not revived)", session.State)
	}
	calls := f.recorder.Calls()
	if len(calls) != 1 || calls[0].trigger != wfmodels.StepTransitionTriggerPRMerged {
		t.Fatalf("history calls = %#v, want one on_pr_merged row", calls)
	}
}

func TestTaskPRWorkflowTrigger_Gates(t *testing.T) {
	valid := func() *github.TaskPR { return linkedTaskPR(1, "merged") }
	cases := []struct {
		name   string
		mutate func(*github.TaskPR)
		want   engine.Trigger
		ok     bool
	}{
		{name: "merged", mutate: func(*github.TaskPR) {}, want: engine.TriggerOnPRMerged, ok: true},
		{name: "merged mixed case", mutate: func(pr *github.TaskPR) { pr.State = " Merged " }, want: engine.TriggerOnPRMerged, ok: true},
		{name: "closed", mutate: func(pr *github.TaskPR) { pr.State = "closed" }, want: engine.TriggerOnPRClosed, ok: true},
		{name: "open", mutate: func(pr *github.TaskPR) { pr.State = "open" }},
		{name: "no task", mutate: func(pr *github.TaskPR) { pr.TaskID = "" }},
		{name: "no owner", mutate: func(pr *github.TaskPR) { pr.Owner = "" }},
		{name: "no repo", mutate: func(pr *github.TaskPR) { pr.Repo = "" }},
		{name: "no number", mutate: func(pr *github.TaskPR) { pr.PRNumber = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr := valid()
			tc.mutate(pr)
			got, ok := taskPRWorkflowTrigger(pr)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("taskPRWorkflowTrigger = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
	if _, ok := taskPRWorkflowTrigger(nil); ok {
		t.Fatal("nil PR must not map to a trigger")
	}
}
