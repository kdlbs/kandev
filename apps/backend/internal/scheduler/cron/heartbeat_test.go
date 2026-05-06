package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// fakeStepLister returns a fixed list of heartbeat steps.
type fakeStepLister struct {
	steps []HeartbeatStepInfo
	err   error
}

func (f *fakeStepLister) ListHeartbeatSteps(_ context.Context) ([]HeartbeatStepInfo, error) {
	return f.steps, f.err
}

// fakeTaskLister returns the tasks per stepID; each call records the
// step it was queried against.
type fakeTaskLister struct {
	byStep   map[string][]HeartbeatTaskInfo
	queried  []string
	err      error
	errSteps map[string]error
}

func (f *fakeTaskLister) ListActiveTasksAtStep(_ context.Context, stepID string) ([]HeartbeatTaskInfo, error) {
	f.queried = append(f.queried, stepID)
	if f.errSteps != nil {
		if e, ok := f.errSteps[stepID]; ok {
			return nil, e
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.byStep[stepID], nil
}

// fakeRuntime returns AllowFire results per agent id.
type fakeRuntime struct {
	allow map[string]bool
	err   error
}

func (f *fakeRuntime) AllowFire(_ context.Context, agentID string, _ time.Time) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.allow == nil {
		return true, nil
	}
	return f.allow[agentID], nil
}

// fakeDispatcher records every fire call.
type fakeDispatcher struct {
	calls []dispatchCall
	err   error
}

type dispatchCall struct {
	taskID  string
	trigger engine.Trigger
	opID    string
	payload any
}

func (f *fakeDispatcher) HandleTrigger(
	_ context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) error {
	f.calls = append(f.calls, dispatchCall{
		taskID: taskID, trigger: trigger, opID: opID, payload: payload,
	})
	return f.err
}

func TestHeartbeatHandler_FiresForEligibleTasks(t *testing.T) {
	steps := &fakeStepLister{
		steps: []HeartbeatStepInfo{{StepID: "s1", WorkflowID: "wf"}},
	}
	tasks := &fakeTaskLister{
		byStep: map[string][]HeartbeatTaskInfo{
			"s1": {
				{TaskID: "t1", WorkflowStepID: "s1", AssigneeAgentProfileID: "a1"},
				{TaskID: "t2", WorkflowStepID: "s1", AssigneeAgentProfileID: "a2"},
			},
		},
	}
	runtime := &fakeRuntime{}
	disp := &fakeDispatcher{}

	now := time.Now().UTC()
	h := NewHeartbeatHandler(steps, tasks, runtime, disp,
		func() time.Time { return now }, logger.Default())

	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 2 {
		t.Fatalf("got %d dispatcher calls, want 2", len(disp.calls))
	}
	for _, c := range disp.calls {
		if c.trigger != engine.TriggerOnHeartbeat {
			t.Errorf("trigger = %q, want on_heartbeat", c.trigger)
		}
		if _, ok := c.payload.(engine.OnHeartbeatPayload); !ok {
			t.Errorf("payload type = %T, want OnHeartbeatPayload", c.payload)
		}
	}
}

func TestHeartbeatHandler_RespectsCooldown(t *testing.T) {
	steps := &fakeStepLister{
		steps: []HeartbeatStepInfo{{StepID: "s1", CadenceSeconds: 60}},
	}
	tasks := &fakeTaskLister{
		byStep: map[string][]HeartbeatTaskInfo{
			"s1": {{TaskID: "t1", WorkflowStepID: "s1", AssigneeAgentProfileID: "a1"}},
		},
	}
	disp := &fakeDispatcher{}

	now := time.Now().UTC()
	clock := now
	h := NewHeartbeatHandler(steps, tasks, &fakeRuntime{}, disp,
		func() time.Time { return clock }, logger.Default())

	// First tick should fire.
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("first tick fires = %d, want 1", len(disp.calls))
	}

	// 30s later — under the 60s cadence — should NOT fire again.
	clock = now.Add(30 * time.Second)
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("under-cadence tick fires = %d, want 1 (cooldown not respected)", len(disp.calls))
	}

	// 90s later — over the cadence — should fire again.
	clock = now.Add(90 * time.Second)
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("third tick: %v", err)
	}
	if len(disp.calls) != 2 {
		t.Fatalf("over-cadence tick fires = %d, want 2", len(disp.calls))
	}
}

func TestHeartbeatHandler_SkipsPausedAgent(t *testing.T) {
	steps := &fakeStepLister{
		steps: []HeartbeatStepInfo{{StepID: "s1"}},
	}
	tasks := &fakeTaskLister{
		byStep: map[string][]HeartbeatTaskInfo{
			"s1": {
				{TaskID: "t-active", AssigneeAgentProfileID: "a-active"},
				{TaskID: "t-paused", AssigneeAgentProfileID: "a-paused"},
			},
		},
	}
	runtime := &fakeRuntime{
		allow: map[string]bool{"a-active": true, "a-paused": false},
	}
	disp := &fakeDispatcher{}

	h := NewHeartbeatHandler(steps, tasks, runtime, disp, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("expected 1 fire (paused agent skipped), got %d", len(disp.calls))
	}
	if disp.calls[0].taskID != "t-active" {
		t.Errorf("fired task = %q, want t-active", disp.calls[0].taskID)
	}
}

func TestHeartbeatHandler_SkipsTaskWithoutAssignee(t *testing.T) {
	steps := &fakeStepLister{
		steps: []HeartbeatStepInfo{{StepID: "s1"}},
	}
	tasks := &fakeTaskLister{
		byStep: map[string][]HeartbeatTaskInfo{
			"s1": {{TaskID: "orphan", AssigneeAgentProfileID: ""}},
		},
	}
	disp := &fakeDispatcher{}

	h := NewHeartbeatHandler(steps, tasks, &fakeRuntime{}, disp, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 0 {
		t.Fatalf("expected 0 fires for unassigned task, got %d", len(disp.calls))
	}
}

func TestHeartbeatHandler_OneStepErrorDoesNotBlockOthers(t *testing.T) {
	steps := &fakeStepLister{
		steps: []HeartbeatStepInfo{{StepID: "bad"}, {StepID: "good"}},
	}
	tasks := &fakeTaskLister{
		byStep: map[string][]HeartbeatTaskInfo{
			"good": {{TaskID: "t-ok", AssigneeAgentProfileID: "a"}},
		},
		errSteps: map[string]error{"bad": errors.New("db down")},
	}
	disp := &fakeDispatcher{}

	h := NewHeartbeatHandler(steps, tasks, &fakeRuntime{}, disp, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 1 || disp.calls[0].taskID != "t-ok" {
		t.Fatalf("unexpected dispatch calls: %+v", disp.calls)
	}
}

func TestHeartbeatHandler_NoStepsIsNoOp(t *testing.T) {
	steps := &fakeStepLister{steps: nil}
	disp := &fakeDispatcher{}
	h := NewHeartbeatHandler(steps, &fakeTaskLister{}, &fakeRuntime{}, disp, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 0 {
		t.Fatalf("expected 0 fires, got %d", len(disp.calls))
	}
}

func TestHeartbeatHandler_NilDependenciesAreNoOp(t *testing.T) {
	h := NewHeartbeatHandler(nil, nil, nil, nil, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}

func TestHeartbeatHandler_OperationIDIncludesTaskAndStepAndTime(t *testing.T) {
	steps := &fakeStepLister{
		steps: []HeartbeatStepInfo{{StepID: "step-x"}},
	}
	tasks := &fakeTaskLister{
		byStep: map[string][]HeartbeatTaskInfo{
			"step-x": {{TaskID: "task-y", AssigneeAgentProfileID: "agent-z"}},
		},
	}
	disp := &fakeDispatcher{}

	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	h := NewHeartbeatHandler(steps, tasks, &fakeRuntime{}, disp,
		func() time.Time { return now }, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("got %d calls", len(disp.calls))
	}
	want := "heartbeat:task-y:step-x:" // suffix is unix seconds
	if got := disp.calls[0].opID; got[:len(want)] != want {
		t.Errorf("op id = %q, want prefix %q", got, want)
	}
}

func TestParseCadenceFromEvents(t *testing.T) {
	cases := []struct {
		name     string
		events   string
		expected int
	}{
		{"empty", ``, 0},
		{"missing key", `{"on_enter":[]}`, 0},
		{"present no config", `{"on_heartbeat":[{"type":"queue_run"}]}`, 0},
		{"with cadence", `{"on_heartbeat":[{"type":"queue_run","config":{"cadence_seconds":120}}]}`, 120},
		{"non-numeric cadence", `{"on_heartbeat":[{"type":"queue_run","config":{"cadence_seconds":"abc"}}]}`, 0},
		{"malformed JSON", `{"on_heartbeat":`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseCadenceFromEvents(tc.events); got != tc.expected {
				t.Errorf("got %d, want %d", got, tc.expected)
			}
		})
	}
}

func TestHasHeartbeatTrigger(t *testing.T) {
	cases := []struct {
		name   string
		events string
		want   bool
	}{
		{"empty", ``, false},
		{"no key", `{"on_enter":[]}`, false},
		{"key present empty", `{"on_heartbeat":[]}`, false},
		{"key present with action", `{"on_heartbeat":[{"type":"queue_run"}]}`, true},
		{"malformed", `{"on_heartbeat":`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasHeartbeatTrigger(tc.events); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
