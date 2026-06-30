package engine

import (
	"context"
	"errors"
	"testing"
)

// fakeRunQueue records every QueueRun call.
type fakeRunQueue struct {
	calls []QueueRunRequest
	err   error
}

func (f *fakeRunQueue) QueueRun(_ context.Context, req QueueRunRequest) error {
	f.calls = append(f.calls, req)
	return f.err
}

// fakePrimary returns a fixed agent profile id for any step.
type fakePrimary struct {
	id         string
	err        error
	taskStepID string
	stepID     *string
	taskID     *string
}

func (f fakePrimary) PrimaryAgentProfileID(_ context.Context, stepID, taskID string) (string, error) {
	if f.stepID != nil {
		*f.stepID = stepID
	}
	if f.taskID != nil {
		*f.taskID = taskID
	}
	return f.id, f.err
}

func (f fakePrimary) PrimaryAgentProfileIDForTask(_ context.Context, taskID string) (string, error) {
	if f.stepID != nil {
		*f.stepID = ""
	}
	if f.taskID != nil {
		*f.taskID = taskID
	}
	return f.id, f.err
}

func (f fakePrimary) WorkflowStepIDForTask(_ context.Context, taskID string) (string, error) {
	if f.taskID != nil {
		*f.taskID = taskID
	}
	return f.taskStepID, f.err
}

// fakeParticipants returns a static slice for any step.
type fakeParticipants struct {
	list       []ParticipantInfo
	err        error
	taskStepID string
	stepID     *string
	taskID     *string
}

func (f fakeParticipants) ListStepParticipants(_ context.Context, stepID, taskID string) ([]ParticipantInfo, error) {
	if f.stepID != nil {
		*f.stepID = stepID
	}
	if f.taskID != nil {
		*f.taskID = taskID
	}
	return f.list, f.err
}

func (f fakeParticipants) WorkflowStepIDForTask(_ context.Context, taskID string) (string, error) {
	if f.taskID != nil {
		*f.taskID = taskID
	}
	return f.taskStepID, f.err
}

// fakeCEO returns a fixed agent profile id (or empty / err).
type fakeCEO struct {
	id  string
	err error
}

func (f fakeCEO) ResolveCEOAgentProfileID(_ context.Context, _ string) (string, error) {
	return f.id, f.err
}

type fakeTaskSteps struct {
	id     string
	err    error
	taskID *string
}

func (f fakeTaskSteps) WorkflowStepIDForTask(_ context.Context, taskID string) (string, error) {
	if f.taskID != nil {
		*f.taskID = taskID
	}
	return f.id, f.err
}

func newQueueRunInput(target, taskID string) ActionInput {
	return ActionInput{
		Trigger: TriggerOnComment,
		State:   MachineState{TaskID: "task-1", SessionID: "sess-1"},
		Step:    StepSpec{ID: "step-1"},
		Action: Action{
			Kind: ActionQueueRun,
			QueueRun: &QueueRunAction{
				Target:  target,
				TaskID:  taskID,
				Reason:  "task_comment",
				Payload: map[string]any{"comment_id": "c-1"},
			},
		},
		OperationID: "op-1",
	}
}

func TestQueueRunCallback_TargetPrimary(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "agent-primary"}}
	if _, err := cb.Execute(context.Background(), newQueueRunInput("primary", "this")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call, got %d", len(q.calls))
	}
	got := q.calls[0]
	if got.AgentProfileID != "agent-primary" {
		t.Fatalf("agent_profile_id = %q, want agent-primary", got.AgentProfileID)
	}
	if got.TaskID != "task-1" {
		t.Fatalf("task_id = %q, want task-1 (resolved from 'this')", got.TaskID)
	}
	if got.WorkflowStepID != "step-1" {
		t.Fatalf("workflow_step_id = %q, want step-1", got.WorkflowStepID)
	}
	if got.Reason != "task_comment" {
		t.Fatalf("reason = %q, want task_comment", got.Reason)
	}
	if got.IdempotencyKey == "" {
		t.Fatalf("expected non-empty idempotency key when OperationID is set")
	}
	if got.Payload["comment_id"] != "c-1" {
		t.Fatalf("payload not propagated: %#v", got.Payload)
	}
}

func TestQueueRunCallback_OnCommentUsesCommentPayloadAndStableKey(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "agent-primary"}}
	in := newQueueRunInput("primary", "this")
	in.OperationID = "task_comment:c-1"
	in.Payload = OnCommentPayload{
		CommentID: "c-1",
		AuthorID:  "user-1",
	}
	in.Action.QueueRun.Payload = nil

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call, got %d", len(q.calls))
	}
	got := q.calls[0]
	if got.IdempotencyKey != "task_comment:c-1" {
		t.Fatalf("idempotency_key = %q, want task_comment:c-1", got.IdempotencyKey)
	}
	if got.Payload["comment_id"] != "c-1" {
		t.Fatalf("payload comment_id = %v, want c-1 (payload=%#v)", got.Payload["comment_id"], got.Payload)
	}
	if got.Payload["author_id"] != "user-1" {
		t.Fatalf("payload author_id = %v, want user-1 (payload=%#v)", got.Payload["author_id"], got.Payload)
	}
}

func TestQueueRunCallback_OnCommentCustomReasonsKeepActionSalt(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "agent-primary"}}

	first := newQueueRunInput("primary", "this")
	first.OperationID = "task_comment:c-1"
	first.Action.QueueRun.Payload = nil
	first.Action.QueueRun.Reason = "follow_up"
	second := newQueueRunInput("primary", "this")
	second.OperationID = "task_comment:c-1"
	second.Action.QueueRun.Payload = nil
	second.Action.QueueRun.Reason = "notify_observer"

	if _, err := cb.Execute(context.Background(), first); err != nil {
		t.Fatalf("first queue_run: %v", err)
	}
	if _, err := cb.Execute(context.Background(), second); err != nil {
		t.Fatalf("second queue_run: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected 2 queue run calls, got %d", len(q.calls))
	}
	for i, call := range q.calls {
		if call.IdempotencyKey == "task_comment:c-1" {
			t.Fatalf("call %d idempotency_key dropped action salt", i)
		}
	}
	if q.calls[0].IdempotencyKey == q.calls[1].IdempotencyKey {
		t.Fatalf("custom reason actions must not share idempotency keys: %#v", q.calls)
	}
}

func TestQueueRunCallback_OnCommentCustomPayloadsKeepActionSalt(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "agent-primary"}}

	first := newQueueRunInput("primary", "this")
	first.OperationID = "task_comment:c-1"
	first.Action.QueueRun.Payload = map[string]any{"source": "first"}
	second := newQueueRunInput("primary", "this")
	second.OperationID = "task_comment:c-1"
	second.Action.QueueRun.Payload = map[string]any{"source": "second"}

	if _, err := cb.Execute(context.Background(), first); err != nil {
		t.Fatalf("first queue_run: %v", err)
	}
	if _, err := cb.Execute(context.Background(), second); err != nil {
		t.Fatalf("second queue_run: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected 2 queue run calls, got %d", len(q.calls))
	}
	for i, call := range q.calls {
		if call.IdempotencyKey == "task_comment:c-1" {
			t.Fatalf("call %d idempotency_key dropped action payload salt", i)
		}
	}
	if q.calls[0].IdempotencyKey == q.calls[1].IdempotencyKey {
		t.Fatalf("custom payload actions must not share idempotency keys: %#v", q.calls)
	}
}

func TestQueueRunCallback_PrimaryUsesResolvedTargetTask(t *testing.T) {
	q := &fakeRunQueue{}
	var resolvedStepID string
	var resolvedTaskID string
	cb := QueueRunCallback{
		Adapter: q,
		Primary: fakePrimary{
			id:         "agent-for-target-task",
			taskStepID: "target-step",
			stepID:     &resolvedStepID,
			taskID:     &resolvedTaskID,
		},
	}
	in := newQueueRunInput("primary", "target-task")
	in.OperationID = "task_comment:c-1"

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call, got %d", len(q.calls))
	}
	if resolvedTaskID != "target-task" {
		t.Fatalf("primary resolved against task %q, want target-task", resolvedTaskID)
	}
	if resolvedStepID != "" {
		t.Fatalf("cross-task primary should use target-task resolver, got step %q", resolvedStepID)
	}
	got := q.calls[0]
	if got.TaskID != "target-task" {
		t.Fatalf("queued task_id = %q, want target-task", got.TaskID)
	}
	if got.WorkflowStepID != "target-step" {
		t.Fatalf("workflow_step_id = %q, want target-step", got.WorkflowStepID)
	}
	if got.AgentProfileID != "agent-for-target-task" {
		t.Fatalf("agent_profile_id = %q, want agent-for-target-task", got.AgentProfileID)
	}
	if got.IdempotencyKey == "task_comment:c-1" {
		t.Fatalf("literal-task primary comment wake must keep action/task/agent salt")
	}
	if got.Payload["comment_id"] != "c-1" {
		t.Fatalf("payload comment_id = %v, want c-1", got.Payload["comment_id"])
	}
	if got.Payload["source_task_id"] != "task-1" {
		t.Fatalf("payload source_task_id = %v, want task-1", got.Payload["source_task_id"])
	}
}

func TestQueueRunCallback_TargetParticipantRole(t *testing.T) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", AgentProfileID: "rev-B"},
		{ID: "p3", Role: "approver", AgentProfileID: "app-A"},
	}}
	cb := QueueRunCallback{Adapter: q, Participants: parts}
	if _, err := cb.Execute(context.Background(), newQueueRunInput("participant_role:reviewer", "")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected 2 queue run calls, got %d", len(q.calls))
	}
	for i, want := range []string{"rev-A", "rev-B"} {
		if q.calls[i].AgentProfileID != want {
			t.Fatalf("call %d agent_profile_id = %q, want %q", i, q.calls[i].AgentProfileID, want)
		}
		if q.calls[i].TaskID != "task-1" {
			t.Fatalf("call %d task_id = %q, want task-1 (resolved from blank)", i, q.calls[i].TaskID)
		}
	}
}

func TestQueueRunCallback_ParticipantRoleUsesResolvedTargetStep(t *testing.T) {
	q := &fakeRunQueue{}
	var listedStepID string
	var listedTaskID string
	parts := fakeParticipants{
		list: []ParticipantInfo{
			{ID: "p-target", Role: "reviewer", AgentProfileID: "rev-target"},
		},
		taskStepID: "target-step",
		stepID:     &listedStepID,
		taskID:     &listedTaskID,
	}
	cb := QueueRunCallback{Adapter: q, Participants: parts}
	in := newQueueRunInput("participant_role:reviewer", "target-task")
	in.OperationID = "task_comment:c-1"

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call, got %d", len(q.calls))
	}
	if listedStepID != "target-step" {
		t.Fatalf("participants listed for step %q, want target-step", listedStepID)
	}
	if listedTaskID != "target-task" {
		t.Fatalf("participants listed for task %q, want target-task", listedTaskID)
	}
	got := q.calls[0]
	if got.WorkflowStepID != "target-step" {
		t.Fatalf("workflow_step_id = %q, want target-step", got.WorkflowStepID)
	}
	if got.TaskID != "target-task" {
		t.Fatalf("task_id = %q, want target-task", got.TaskID)
	}
	if got.AgentProfileID != "rev-target" {
		t.Fatalf("agent_profile_id = %q, want rev-target", got.AgentProfileID)
	}
}

func TestQueueRunCallback_OnCommentParticipantRoleKeepsPerAgentKeys(t *testing.T) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", AgentProfileID: "rev-B"},
	}}
	cb := QueueRunCallback{Adapter: q, Participants: parts}
	in := newQueueRunInput("participant_role:reviewer", "")
	in.OperationID = "task_comment:c-1"
	in.Payload = OnCommentPayload{CommentID: "c-1", AuthorID: "user-1"}
	in.Action.QueueRun.Payload = nil

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected 2 queue run calls, got %d", len(q.calls))
	}
	if q.calls[0].IdempotencyKey == q.calls[1].IdempotencyKey {
		t.Fatalf("fan-out idempotency keys must be per-agent: %#v", q.calls)
	}
	for i, call := range q.calls {
		if call.IdempotencyKey == "task_comment:c-1" {
			t.Fatalf("call %d idempotency_key dropped per-agent salt", i)
		}
		if call.Payload["comment_id"] != "c-1" || call.Payload["author_id"] != "user-1" {
			t.Fatalf("call %d payload missing comment identity: %#v", i, call.Payload)
		}
	}
}

func TestQueueRunCallback_TargetSpecificAgent(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, TaskSteps: fakeTaskSteps{id: "target-step"}}
	if _, err := cb.Execute(context.Background(), newQueueRunInput("agent_profile_id:some-agent", "task-2")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	if q.calls[0].AgentProfileID != "some-agent" {
		t.Fatalf("agent_profile_id = %q", q.calls[0].AgentProfileID)
	}
	if q.calls[0].TaskID != "task-2" {
		t.Fatalf("task_id = %q, want task-2 (literal)", q.calls[0].TaskID)
	}
	if q.calls[0].WorkflowStepID != "target-step" {
		t.Fatalf("workflow_step_id = %q, want target-step", q.calls[0].WorkflowStepID)
	}
}

func TestQueueRunCallback_TargetWorkspaceCEO(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, CEOResolver: fakeCEO{id: "ceo-agent"}}
	if _, err := cb.Execute(context.Background(), newQueueRunInput("workspace.ceo_agent", "this")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	if q.calls[0].AgentProfileID != "ceo-agent" {
		t.Fatalf("agent_profile_id = %q, want ceo-agent", q.calls[0].AgentProfileID)
	}
}

func TestQueueRunCallback_TargetWorkspaceCEOUsesResolvedTargetStep(t *testing.T) {
	q := &fakeRunQueue{}
	var resolvedTaskID string
	cb := QueueRunCallback{
		Adapter:     q,
		CEOResolver: fakeCEO{id: "ceo-agent"},
		TaskSteps:   fakeTaskSteps{id: "target-step", taskID: &resolvedTaskID},
	}
	in := newQueueRunInput("workspace.ceo_agent", "target-task")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	if resolvedTaskID != "target-task" {
		t.Fatalf("target step resolved for task %q, want target-task", resolvedTaskID)
	}
	got := q.calls[0]
	if got.TaskID != "target-task" {
		t.Fatalf("task_id = %q, want target-task", got.TaskID)
	}
	if got.WorkflowStepID != "target-step" {
		t.Fatalf("workflow_step_id = %q, want target-step", got.WorkflowStepID)
	}
	if got.AgentProfileID != "ceo-agent" {
		t.Fatalf("agent_profile_id = %q, want ceo-agent", got.AgentProfileID)
	}
}

func TestQueueRunCallback_MissingAdapter_Errors(t *testing.T) {
	cb := QueueRunCallback{} // no Adapter
	_, err := cb.Execute(context.Background(), newQueueRunInput("primary", "this"))
	if err == nil || !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got %v", err)
	}
}

func TestQueueRunCallback_UnknownTarget_Errors(t *testing.T) {
	cb := QueueRunCallback{Adapter: &fakeRunQueue{}}
	_, err := cb.Execute(context.Background(), newQueueRunInput("nonsense_target", "this"))
	if err == nil {
		t.Fatalf("expected error for unknown target")
	}
}

func TestQueueRunCallback_TargetPrimaryNoResolver_Errors(t *testing.T) {
	cb := QueueRunCallback{Adapter: &fakeRunQueue{}} // missing Primary
	_, err := cb.Execute(context.Background(), newQueueRunInput("primary", "this"))
	if err == nil || !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got %v", err)
	}
}

func TestQueueRunCallback_ParticipantRoleNoStore_Errors(t *testing.T) {
	cb := QueueRunCallback{Adapter: &fakeRunQueue{}}
	_, err := cb.Execute(context.Background(), newQueueRunInput("participant_role:reviewer", "this"))
	if err == nil || !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got %v", err)
	}
}

func TestQueueRunCallback_CEONoResolver_Errors(t *testing.T) {
	cb := QueueRunCallback{Adapter: &fakeRunQueue{}}
	_, err := cb.Execute(context.Background(), newQueueRunInput("workspace.ceo_agent", "this"))
	if err == nil || !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got %v", err)
	}
}

func TestQueueRunCallback_TaskIDResolution(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "task-1"},
		{"this", "task-1"},
		{"task-99", "task-99"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			q := &fakeRunQueue{}
			cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "p", taskStepID: "target-step"}}
			if _, err := cb.Execute(context.Background(), newQueueRunInput("primary", tc.input)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := q.calls[0].TaskID; got != tc.want {
				t.Fatalf("TaskID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQueueRunForEachParticipantCallback_FansOut(t *testing.T) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", AgentProfileID: "rev-B"},
		{ID: "p3", Role: "watcher", AgentProfileID: "watch-A"},
	}}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger:     TriggerOnEnter,
		State:       MachineState{TaskID: "task-1"},
		Step:        StepSpec{ID: "step-1"},
		OperationID: "op-1",
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:   "reviewer",
				Reason: "review_started",
			},
		},
	}
	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected 2 fan-out calls, got %d", len(q.calls))
	}
	for i, want := range []string{"rev-A", "rev-B"} {
		if q.calls[i].AgentProfileID != want {
			t.Fatalf("call %d agent_profile_id = %q, want %q", i, q.calls[i].AgentProfileID, want)
		}
		if q.calls[i].Reason != "review_started" {
			t.Fatalf("call %d reason = %q", i, q.calls[i].Reason)
		}
	}
}

func TestQueueRunForEachParticipantCallback_OnCommentKeepsPerAgentKeys(t *testing.T) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", AgentProfileID: "rev-B"},
	}}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger:     TriggerOnComment,
		State:       MachineState{TaskID: "task-1"},
		Step:        StepSpec{ID: "step-1"},
		OperationID: "task_comment:c-1",
		Payload:     OnCommentPayload{CommentID: "c-1", AuthorID: "user-1"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:   "reviewer",
				Reason: "task_comment",
			},
		},
	}

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected 2 fan-out calls, got %d", len(q.calls))
	}
	if q.calls[0].IdempotencyKey == q.calls[1].IdempotencyKey {
		t.Fatalf("fan-out idempotency keys must be per-agent: %#v", q.calls)
	}
	for i, call := range q.calls {
		if call.IdempotencyKey == "task_comment:c-1" {
			t.Fatalf("call %d idempotency_key dropped per-agent salt", i)
		}
		if call.Payload["comment_id"] != "c-1" || call.Payload["author_id"] != "user-1" {
			t.Fatalf("call %d payload missing comment identity: %#v", i, call.Payload)
		}
	}
}

func TestQueueRunForEachParticipantCallback_DifferentActionsKeepActionSalt(t *testing.T) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "same-agent"},
		{ID: "p2", Role: "observer", AgentProfileID: "same-agent"},
	}}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	base := ActionInput{
		Trigger:     TriggerOnComment,
		State:       MachineState{TaskID: "task-1"},
		Step:        StepSpec{ID: "step-1"},
		OperationID: "task_comment:c-1",
		Payload:     OnCommentPayload{CommentID: "c-1", AuthorID: "user-1"},
	}
	first := base
	first.Action = Action{
		Kind: ActionQueueRunForEachParticipant,
		QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
			Role:    "reviewer",
			Reason:  "follow_up",
			Payload: map[string]any{"source": "reviewer"},
		},
	}
	second := base
	second.Action = Action{
		Kind: ActionQueueRunForEachParticipant,
		QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
			Role:    "observer",
			Reason:  "follow_up",
			Payload: map[string]any{"source": "observer"},
		},
	}

	if _, err := cb.Execute(context.Background(), first); err != nil {
		t.Fatalf("first queue_run_for_each_participant: %v", err)
	}
	if _, err := cb.Execute(context.Background(), second); err != nil {
		t.Fatalf("second queue_run_for_each_participant: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected 2 fan-out calls, got %d", len(q.calls))
	}
	if q.calls[0].AgentProfileID != "same-agent" || q.calls[1].AgentProfileID != "same-agent" {
		t.Fatalf("test setup expected both actions to target same agent: %#v", q.calls)
	}
	if q.calls[0].IdempotencyKey == q.calls[1].IdempotencyKey {
		t.Fatalf("different for-each action configs must not share idempotency keys: %#v", q.calls)
	}
}

func TestQueueRunForEachParticipantCallback_NoMatchingRole_NoCalls(t *testing.T) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p3", Role: "watcher", AgentProfileID: "watch-A"},
	}}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Step: StepSpec{ID: "step-1"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role: "approver",
			},
		},
	}
	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 0 {
		t.Fatalf("expected no calls when no participant matches, got %d", len(q.calls))
	}
}

func TestQueueRunForEachParticipantCallback_MissingDeps_Errors(t *testing.T) {
	cb := QueueRunForEachParticipantCallback{} // no deps
	_, err := cb.Execute(context.Background(), ActionInput{
		Action: Action{
			Kind:                       ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{Role: "reviewer"},
		},
	})
	if err == nil || !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got %v", err)
	}
}
