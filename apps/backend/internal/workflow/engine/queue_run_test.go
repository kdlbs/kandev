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
	id  string
	err error
}

func (f fakePrimary) PrimaryAgentProfileID(_ context.Context, _ string) (string, error) {
	return f.id, f.err
}

// fakeParticipants returns a static slice for any step.
type fakeParticipants struct {
	list []ParticipantInfo
	err  error
}

func (f fakeParticipants) ListStepParticipants(_ context.Context, _, _ string) ([]ParticipantInfo, error) {
	return f.list, f.err
}

// fakeCEO returns a fixed agent profile id (or empty / err).
type fakeCEO struct {
	id  string
	err error
}

func (f fakeCEO) ResolveCEOAgentProfileID(_ context.Context, _ string) (string, error) {
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

func TestQueueRunCallback_TargetSpecificAgent(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q}
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
			cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "p"}}
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
