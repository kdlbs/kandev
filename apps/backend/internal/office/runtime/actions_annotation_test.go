package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/shared"
)

// annotationTaskCreator is a minimal TaskCreator fake for the annotation
// predicate: it can report a task's workspace and, unlike
// recordingTaskCreator, can distinguish "no such task" from "lookup
// failed".
type annotationTaskCreator struct {
	workspaces map[string]string
	lookupErr  error
	lookups    []string
}

func (c *annotationTaskCreator) GetTaskWorkspaceID(_ context.Context, taskID string) (string, error) {
	c.lookups = append(c.lookups, taskID)
	if c.lookupErr != nil {
		return "", c.lookupErr
	}
	return c.workspaces[taskID], nil
}

func (c *annotationTaskCreator) GetTaskProjectID(context.Context, string) (string, error) {
	return "", nil
}
func (c *annotationTaskCreator) CreateOfficeTaskAsAgent(
	context.Context, string, string, string, string, string, string,
) (string, error) {
	return "", nil
}
func (c *annotationTaskCreator) CreateOfficeSubtaskAsAgent(
	context.Context, string, string, string, string, string,
) (string, error) {
	return "", nil
}

func tasklessAnnotationRunCtx(workspaceID string, tasks *annotationTaskCreator) (RunContext, *Actions) {
	runCtx, actions, _ := tasklessAnnotationRunCtxWithWriter(workspaceID, tasks)
	return runCtx, actions
}

func tasklessAnnotationRunCtxWithWriter(
	workspaceID string, tasks *annotationTaskCreator,
) (RunContext, *Actions, *recordingCommentWriter) {
	runCtx := RunContext{
		AgentID:     "agent-1",
		WorkspaceID: workspaceID,
		TaskID:      "",
		RunID:       "run-1",
		Capabilities: Capabilities{
			CanPostComments: true,
		},
	}
	writer := &recordingCommentWriter{}
	actions := NewActions(ActionDependencies{Comments: writer, Tasks: tasks})
	return runCtx, actions, writer
}

func TestPostComment_TasklessRunAnnotatesTaskInWorkspace(t *testing.T) {
	tasks := &annotationTaskCreator{workspaces: map[string]string{"task-x": "ws-1"}}
	runCtx, actions := tasklessAnnotationRunCtx("ws-1", tasks)

	if err := actions.PostComment(context.Background(), runCtx, "task-x", "blocker found"); err != nil {
		t.Fatalf("PostComment: %v", err)
	}
}

func TestPostComment_TasklessRunRefusesCrossWorkspaceTarget(t *testing.T) {
	tasks := &annotationTaskCreator{workspaces: map[string]string{"task-x": "ws-2"}}
	runCtx, actions := tasklessAnnotationRunCtx("ws-1", tasks)

	err := actions.PostComment(context.Background(), runCtx, "task-x", "blocker found")
	if !errors.Is(err, ErrTaskOutOfScope) {
		t.Fatalf("error = %v, want ErrTaskOutOfScope", err)
	}
}

func TestPostComment_TasklessRunRefusesNonexistentTargetWithSameSentinelAsCrossWorkspace(t *testing.T) {
	tasks := &annotationTaskCreator{workspaces: map[string]string{}}
	runCtx, actions := tasklessAnnotationRunCtx("ws-1", tasks)

	err := actions.PostComment(context.Background(), runCtx, "no-such-task", "blocker found")
	if !errors.Is(err, ErrTaskOutOfScope) {
		t.Fatalf("error = %v, want ErrTaskOutOfScope (anti-oracle: same sentinel as cross-workspace)", err)
	}
}

func TestPostComment_TasklessRunFailedLookupReturnsErrorNotRefusal(t *testing.T) {
	lookupErr := errors.New("db unavailable")
	tasks := &annotationTaskCreator{lookupErr: lookupErr}
	runCtx, actions := tasklessAnnotationRunCtx("ws-1", tasks)

	err := actions.PostComment(context.Background(), runCtx, "task-x", "blocker found")
	if !errors.Is(err, lookupErr) {
		t.Fatalf("error = %v, want the raw lookup error surfaced (not a 403 refusal)", err)
	}
	if errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a failed lookup must not be reported as a forbidden refusal: %v", err)
	}
}

func TestPostComment_TasklessRunEmptyWorkspaceClaimRefusesBeforeResolvingTarget(t *testing.T) {
	tasks := &annotationTaskCreator{workspaces: map[string]string{"task-x": ""}}
	runCtx, actions := tasklessAnnotationRunCtx("", tasks)

	err := actions.PostComment(context.Background(), runCtx, "task-x", "blocker found")
	if !errors.Is(err, ErrWorkspaceOutOfScope) {
		t.Fatalf("error = %v, want ErrWorkspaceOutOfScope", err)
	}
	if len(tasks.lookups) != 0 {
		t.Fatalf("target should not be resolved when the run's own workspace claim is empty: %v", tasks.lookups)
	}
}

func TestPostComment_TaskBoundRunUnaffectedByEmptyWorkspaceClaim(t *testing.T) {
	// A task-bound run's annotation predicate never reads runCtx.WorkspaceID,
	// so an empty claim (which would refuse a taskless run) has no effect.
	comments := &recordingCommentWriter{}
	actions := NewActions(ActionDependencies{Comments: comments})
	runCtx := RunContext{
		AgentID:      "agent-1",
		WorkspaceID:  "",
		TaskID:       "task-1",
		RunID:        "run-1",
		Capabilities: Capabilities{CanPostComments: true},
	}

	if err := actions.PostComment(context.Background(), runCtx, "task-1", "status update"); err != nil {
		t.Fatalf("PostComment: %v", err)
	}
	if len(comments.comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments.comments))
	}
}

func TestPostComment_TaskBoundRunRefusesAnyOtherTarget(t *testing.T) {
	tasks := &annotationTaskCreator{workspaces: map[string]string{"task-2": "ws-1"}}
	comments := &recordingCommentWriter{}
	actions := NewActions(ActionDependencies{Comments: comments, Tasks: tasks})
	runCtx := RunContext{
		AgentID:      "agent-1",
		WorkspaceID:  "ws-1",
		TaskID:       "task-1",
		RunID:        "run-1",
		Capabilities: Capabilities{CanPostComments: true},
	}

	err := actions.PostComment(context.Background(), runCtx, "task-2", "hello")
	if !errors.Is(err, ErrTaskOutOfScope) {
		t.Fatalf("error = %v, want ErrTaskOutOfScope", err)
	}
	if len(tasks.lookups) != 0 {
		t.Fatalf("task-bound annotation must not consult workspace lookups: %v", tasks.lookups)
	}
}

func TestPostComment_EmptyBodyRefusedBeforeBranch(t *testing.T) {
	tasks := &annotationTaskCreator{}
	comments := &recordingCommentWriter{}
	actions := NewActions(ActionDependencies{Comments: comments, Tasks: tasks})
	runCtx := RunContext{
		AgentID:      "agent-1",
		WorkspaceID:  "ws-1",
		RunID:        "run-1",
		Capabilities: Capabilities{CanPostComments: true},
	}

	err := actions.PostComment(context.Background(), runCtx, "task-x", "   ")
	if !errors.Is(err, ErrCommentBodyRequired) {
		t.Fatalf("error = %v, want ErrCommentBodyRequired", err)
	}
	if len(tasks.lookups) != 0 {
		t.Fatalf("body validation must precede target resolution: %v", tasks.lookups)
	}
}

func TestPostComment_NoTargetAndNoBoundTaskRefuses(t *testing.T) {
	tasks := &annotationTaskCreator{workspaces: map[string]string{}}
	runCtx, actions := tasklessAnnotationRunCtx("ws-1", tasks)

	err := actions.PostComment(context.Background(), runCtx, "", "blocker found")
	if !errors.Is(err, ErrTaskOutOfScope) {
		t.Fatalf("error = %v, want ErrTaskOutOfScope", err)
	}
}

func TestPostComment_AnnotationIsNotIdempotent(t *testing.T) {
	tasks := &annotationTaskCreator{workspaces: map[string]string{"task-x": "ws-1"}}
	runCtx, actions, writer := tasklessAnnotationRunCtxWithWriter("ws-1", tasks)

	for i := 0; i < 2; i++ {
		if err := actions.PostComment(context.Background(), runCtx, "task-x", "same body"); err != nil {
			t.Fatalf("PostComment call %d: %v", i, err)
		}
	}
	if len(writer.comments) != 2 {
		t.Fatalf("expected 2 recorded comments (no dedup), got %d", len(writer.comments))
	}
}
