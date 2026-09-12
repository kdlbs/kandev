package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type canvasSourceLookupStub struct {
	tasks    map[string]*models.Task
	sessions map[string]*models.TaskSession
	err      error
}

func (s canvasSourceLookupStub) GetTask(_ context.Context, id string) (*models.Task, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.tasks[id], nil
}

func (s canvasSourceLookupStub) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sessions[id], nil
}

func TestProjectCanvasSourceLabelsRequiresCurrentWorkspaceAccess(t *testing.T) {
	lookup := canvasSourceLookupStub{
		tasks: map[string]*models.Task{
			"task-1": {ID: "task-1", WorkspaceID: "workspace-1", Title: "Canvas source task"},
			"task-2": {ID: "task-2", WorkspaceID: "workspace-2", Title: "Foreign task"},
		},
		sessions: map[string]*models.TaskSession{
			"session-1": {ID: "session-1", TaskID: "task-1", Name: "Canvas source session"},
			"session-2": {ID: "session-2", TaskID: "task-2", Name: "Foreign session"},
		},
	}

	if got := projectCanvasSourceLabels(context.Background(), lookup, "workspace-1", "task-1", "session-1"); got != (canvasSourceLabels{TaskTitle: "Canvas source task", SessionName: "Canvas source session"}) {
		t.Fatalf("authorized labels = %+v", got)
	}
	if got := projectCanvasSourceLabels(context.Background(), lookup, "workspace-1", "task-2", "session-2"); got != (canvasSourceLabels{}) {
		t.Fatalf("foreign labels = %+v, want empty", got)
	}
	if got := projectCanvasSourceLabels(context.Background(), lookup, "workspace-1", "missing-task", "missing-session"); got != (canvasSourceLabels{}) {
		t.Fatalf("deleted labels = %+v, want empty", got)
	}
}

func TestProjectCanvasSourceLabelsRejectsMismatchedSession(t *testing.T) {
	lookup := canvasSourceLookupStub{
		tasks: map[string]*models.Task{
			"task-1": {ID: "task-1", WorkspaceID: "workspace-1", Title: "Canvas source task"},
		},
		sessions: map[string]*models.TaskSession{
			"session-1": {ID: "session-1", TaskID: "other-task", Name: "Wrong session"},
		},
	}

	got := projectCanvasSourceLabels(context.Background(), lookup, "workspace-1", "task-1", "session-1")
	if got.TaskTitle != "Canvas source task" || got.SessionName != "" {
		t.Fatalf("mismatched session labels = %+v", got)
	}
}

func TestProjectCanvasSourceLabelsFailsClosedOnLookupError(t *testing.T) {
	got := projectCanvasSourceLabels(
		context.Background(),
		canvasSourceLookupStub{err: errors.New("lookup failed")},
		"workspace-1",
		"task-1",
		"session-1",
	)
	if got != (canvasSourceLabels{}) {
		t.Fatalf("error labels = %+v, want empty", got)
	}
}
