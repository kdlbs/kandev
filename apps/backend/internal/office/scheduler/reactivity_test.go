package scheduler

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// recordedQueueCall captures one queue(agentID, RunContext) invocation from
// reactToComment for assertion.
type recordedQueueCall struct {
	agentID string
	ctx     RunContext
}

// TestReactToComment_SelfCommentDoesNotWakeAssignee is the card's required
// regression: a comment authored by the task's own assignee (author_type
// "agent", author_id == the assignee's agent profile id) must NOT queue a
// task_comment run for that assignee. This is the guard in reactToComment
// (reactivity.go); it only works when the comment's author_id was
// persisted as the acting agent's real profile id (dashboard's
// createComment handler is the thing that used to break this by hardcoding
// author_id="user").
func TestReactToComment_SelfCommentDoesNotWakeAssignee(t *testing.T) {
	ss := &SchedulerService{logger: logger.Default()}
	task := &TaskSnapshot{
		ID:                     "task-1",
		WorkspaceID:            "ws-1",
		State:                  "IN_PROGRESS",
		AssigneeAgentProfileID: "agent-a",
	}
	comment := &MutationComment{
		ID:         "comment-1",
		Body:       "Landscape scan complete, no action needed.",
		AuthorType: "agent",
		AuthorID:   "agent-a",
	}

	var calls []recordedQueueCall
	queue := func(agentID string, c RunContext) {
		calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
	}

	ss.reactToComment(context.Background(), task, comment, queue)

	for _, c := range calls {
		if c.agentID == "agent-a" && c.ctx.Reason == RunReasonTaskComment {
			t.Fatalf("agent-a was woken by its own comment: %+v", c)
		}
	}
}

// TestReactToComment_OtherAuthorWakesAssignee is the positive counterpart:
// a comment from a different agent (or the user) on a task assigned to
// agent-b DOES queue a task_comment run for agent-b. Preserving this is
// explicitly required by the card — the fix must not suppress cross-agent
// wakes while closing the self-wake hole.
func TestReactToComment_OtherAuthorWakesAssignee(t *testing.T) {
	ss := &SchedulerService{logger: logger.Default()}
	task := &TaskSnapshot{
		ID:                     "task-2",
		WorkspaceID:            "ws-1",
		State:                  "IN_PROGRESS",
		AssigneeAgentProfileID: "agent-b",
	}
	comment := &MutationComment{
		ID:         "comment-2",
		Body:       "Please take a look at this.",
		AuthorType: "agent",
		AuthorID:   "agent-a",
	}

	var calls []recordedQueueCall
	queue := func(agentID string, c RunContext) {
		calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
	}

	ss.reactToComment(context.Background(), task, comment, queue)

	found := false
	for _, c := range calls {
		if c.agentID == "agent-b" && c.ctx.Reason == RunReasonTaskComment {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected agent-b to be queued a task_comment run, got calls: %+v", calls)
	}
}
