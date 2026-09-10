package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestActionsCreateSubtaskFailsClosedWhenParentProjectLookupErrors(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "child-1", taskScopes: map[string]taskScope{
		"parent-1": {WorkspaceID: "ws-1", ProjectID: "project-1"},
	}}
	lookupErr := errors.New("get parent task project: transient repository failure")
	creator.projectLookupErr = lookupErr
	projects := &recordingProjectManager{projects: []*models.Project{
		{ID: "project-1", WorkspaceID: "ws-1"},
	}}
	actions := NewActions(ActionDependencies{Tasks: creator, Projects: projects})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1", TaskID: "task-1",
		Capabilities: Capabilities{
			CanCreateSubtasks: true,
			AllowedTaskIDs:    []string{"parent-1"},
		},
	}

	_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "child", ParentTaskID: "parent-1", ProjectID: "project-1",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("error = %v, want the GetTaskProjectID lookup error surfaced rather than treated as "+
			"'workspace out of scope'", err)
	}
	if errors.Is(err, ErrWorkspaceOutOfScope) {
		t.Fatalf("error = %v, must not mask the lookup failure as a scope denial", err)
	}
	if len(creator.calls) != 0 {
		t.Fatalf("creator called despite parent project lookup error: %#v", creator.calls)
	}
}

func TestActionsCreateSubtaskFailsClosedWhenParentWorkspaceLookupErrors(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "child-1", taskScopes: map[string]taskScope{
		"parent-1": {WorkspaceID: "ws-1"},
	}}
	lookupErr := errors.New("get parent task workspace: transient repository failure")
	creator.workspaceLookupErr = lookupErr
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1", TaskID: "task-1",
		Capabilities: Capabilities{
			CanCreateSubtasks: true,
			AllowedTaskIDs:    []string{"parent-1"},
		},
	}

	_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "child", ParentTaskID: "parent-1",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("error = %v, want the GetTaskWorkspaceID lookup error surfaced rather than treated as "+
			"'workspace out of scope'", err)
	}
	if errors.Is(err, ErrWorkspaceOutOfScope) {
		t.Fatalf("error = %v, must not mask the lookup failure as a scope denial", err)
	}
	if len(creator.calls) != 0 {
		t.Fatalf("creator called despite parent workspace lookup error: %#v", creator.calls)
	}
}
