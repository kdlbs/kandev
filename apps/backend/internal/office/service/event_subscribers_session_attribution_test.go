package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
)

// These tests use DISTINCT assignee and acting-agent profiles, the blind
// spot that let the assignee-attribution bug ship: any test where the two
// are the same profile passes whether or not the session lookup runs.

// TestAutoPostAgentComment_AttributesActingAgentNotAssignee pins the core
// regression: a session-bridged comment must be authored by the agent that
// ran the session (resolved from task_sessions.agent_profile_id), not the
// task's assignee.
func TestAutoPostAgentComment_AttributesActingAgentNotAssignee(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "runner-pm")
	createTestAgent(t, svc, "ws-1", "critic")
	taskID := createOfficeTask(t, svc, "ws-1", "runner-pm")
	insertTestTaskSession(t, svc, "sess-critic", taskID, "critic")

	event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
		"task_id":    taskID,
		"session_id": "sess-critic",
		"agent_text": "Rejected: the note isn't retrievable.",
		"agent_id":   "exec-1234", // execution id, must NOT be used as author
	})
	if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	comments, err := svc.ListComments(ctx, taskID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	for _, c := range comments {
		if c.Source == "session" && c.Body == "Rejected: the note isn't retrievable." {
			if c.AuthorID != "critic" {
				t.Fatalf("author_id = %q, want %q (the agent that actually spoke)", c.AuthorID, "critic")
			}
			return
		}
	}
	t.Fatalf("expected session comment, got %+v", comments)
}

// TestAutoPostAgentComment_FallsBackToAssigneeWhenSessionUnresolvable pins
// the no-regression case: when the session row can't be resolved to an
// agent (missing task_sessions row), the bridge still falls back to the
// assignee instead of dropping the comment or leaving it unattributed.
func TestAutoPostAgentComment_FallsBackToAssigneeWhenSessionUnresolvable(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "runner-pm2")
	taskID := createOfficeTask(t, svc, "ws-1", "runner-pm2")

	event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
		"task_id":    taskID,
		"session_id": "sess-missing",
		"agent_text": "Note delivered above.",
		"agent_id":   "exec-5678",
	})
	if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	comments, err := svc.ListComments(ctx, taskID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	for _, c := range comments {
		if c.Source == "session" && c.Body == "Note delivered above." {
			if c.AuthorID != "runner-pm2" {
				t.Fatalf("author_id = %q, want %q (fallback to assignee)", c.AuthorID, "runner-pm2")
			}
			return
		}
	}
	t.Fatalf("expected session comment, got %+v", comments)
}

// TestAutoPostAgentComment_NonAssigneeAuthorQueuesNoRun pins the coupling
// documented at queueCommentRun: a session-bridged mirror of a turn must
// never itself queue a run, even once its author is the acting (non-
// assignee) agent. Before the fix this held by accident, because the
// bridge always wrote the assignee as author and tripped the self-comment
// gate; fixing the author alone (without the explicit source guard) would
// start waking the runner on every reviewer/approver turn.
func TestAutoPostAgentComment_NonAssigneeAuthorQueuesNoRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "runner-pm3")
	createTestAgent(t, svc, "ws-1", "critic3")
	taskID := createOfficeTask(t, svc, "ws-1", "runner-pm3")
	insertTestTaskSession(t, svc, "sess-critic3", taskID, "critic3")

	event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
		"task_id":    taskID,
		"session_id": "sess-critic3",
		"agent_text": "Approved. The transition was applied.",
		"agent_id":   "exec-9999",
	})
	if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.Reason == service.RunReasonTaskComment {
			t.Fatalf("session-bridged comment must not queue a run, got %+v", run)
		}
	}
}

// TestAutoPostAgentComment_RecordsSuccessForActingAgent pins that a
// successful turn's consecutive-failure reset credits the agent that
// actually spoke, not the assignee — the same resolved identity used for
// attribution (event_subscribers.go), not a second, independently-wrong
// value.
func TestAutoPostAgentComment_RecordsSuccessForActingAgent(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "runner-pm4")
	createTestAgent(t, svc, "ws-1", "critic4")

	// Give both agents prior failures so a reset is observable and
	// asymmetry (only the acting agent resets) is provable.
	for i := 0; i < 2; i++ {
		taskID := uuidish("task-fail-runner", i)
		insertSyntheticTask(t, svc, taskID, "ws-1", "runner-pm4")
		w := queueAndReadRun(t, svc, "runner-pm4", taskID)
		if err := svc.HandleAgentFailure(ctx, w, "boom"); err != nil {
			t.Fatalf("handle failure (runner) %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		taskID := uuidish("task-fail-critic", i)
		insertSyntheticTask(t, svc, taskID, "ws-1", "critic4")
		w := queueAndReadRun(t, svc, "critic4", taskID)
		if err := svc.HandleAgentFailure(ctx, w, "boom"); err != nil {
			t.Fatalf("handle failure (critic) %d: %v", i, err)
		}
	}

	taskID := createOfficeTask(t, svc, "ws-1", "runner-pm4")
	insertTestTaskSession(t, svc, "sess-critic4", taskID, "critic4")

	event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
		"task_id":    taskID,
		"session_id": "sess-critic4",
		"agent_text": "Approved. All checks satisfied.",
		"agent_id":   "exec-4321",
	})
	if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	critic, err := svc.GetAgentInstance(ctx, "critic4")
	if err != nil {
		t.Fatalf("get critic: %v", err)
	}
	if critic.ConsecutiveFailures != 0 {
		t.Fatalf("critic4 consecutive failures = %d, want 0 (acting agent's turn succeeded)", critic.ConsecutiveFailures)
	}

	runner, err := svc.GetAgentInstance(ctx, "runner-pm4")
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if runner.ConsecutiveFailures != 2 {
		t.Fatalf("runner-pm4 consecutive failures = %d, want 2 (assignee did not act, must not be credited)", runner.ConsecutiveFailures)
	}
}
