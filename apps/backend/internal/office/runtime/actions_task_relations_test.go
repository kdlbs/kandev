package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// recordingTaskReads is the TaskReadRelation test double: it admits exactly
// the (caller, target) pairs in allowed and records every question asked.
type recordingTaskReads struct {
	allowed map[[2]string]bool
	err     error
	asked   [][2]string
}

func (r *recordingTaskReads) CanReadTask(_ context.Context, currentTaskID, targetTaskID string) (bool, error) {
	pair := [2]string{currentTaskID, targetTaskID}
	r.asked = append(r.asked, pair)
	if r.err != nil {
		return false, r.err
	}
	return r.allowed[pair], nil
}

func relationsRunCtx(allowed ...string) RunContext {
	return RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1", TaskID: "task-1", RunID: "run-1",
		Capabilities: Capabilities{
			CanCreateTasks:    true,
			CanCreateSubtasks: true,
			AllowedTaskIDs:    allowed,
		},
	}
}

func relationsCreator(scopes map[string]string) *recordingTaskCreator {
	taskScopes := map[string]taskScope{"task-1": {WorkspaceID: "ws-1", ProjectID: "project-1"}}
	for id, workspaceID := range scopes {
		taskScopes[id] = taskScope{WorkspaceID: workspaceID, ProjectID: "project-1"}
	}
	return &recordingTaskCreator{taskID: "created", taskScopes: taskScopes}
}

// relationsActions wires the dependencies a root create needs: the run task's
// project resolves to project-1, which the project manager owns in ws-1.
func relationsActions(creator *recordingTaskCreator, reads TaskReadRelation) *Actions {
	return NewActions(ActionDependencies{
		Tasks:     creator,
		TaskReads: reads,
		Projects:  &recordingProjectManager{projects: []*models.Project{{ID: "project-1", WorkspaceID: "ws-1"}}},
	})
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.1
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.5
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.6
func TestActionsCreateTaskForwardsRootRelations(t *testing.T) {
	creator := relationsCreator(map[string]string{"task-1": "ws-1", "dep-2": "ws-1"})
	actions := relationsActions(creator, nil)

	_, err := actions.CreateTask(context.Background(), relationsRunCtx("dep-2"), CreateTaskInput{
		Title:         "Join",
		BlockedBy:     []string{" task-1 ", "dep-2", "task-1"},
		WorkspaceMode: " new_workspace ",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("creator calls = %d, want 1", len(creator.calls))
	}
	want := shared.AgentTaskRelations{BlockedBy: []string{"task-1", "dep-2"}, WorkspaceMode: "new_workspace"}
	assertRelations(t, creator.calls[0].Relations, want)
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.1
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.2
func TestActionsCreateTaskForwardsChildRelations(t *testing.T) {
	for _, mode := range []string{"new_workspace", "inherit_parent"} {
		t.Run(mode, func(t *testing.T) {
			creator := relationsCreator(map[string]string{"parent-1": "ws-1", "task-1": "ws-1"})
			actions := relationsActions(creator, nil)

			_, err := actions.CreateTask(context.Background(), relationsRunCtx("parent-1"), CreateTaskInput{
				Title: "Batch", ParentTaskID: "parent-1", BlockedBy: []string{"task-1"}, WorkspaceMode: mode,
			})
			if err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			if len(creator.calls) != 1 || creator.calls[0].ParentTaskID != "parent-1" {
				t.Fatalf("creator calls = %#v", creator.calls)
			}
			assertRelations(t, creator.calls[0].Relations,
				shared.AgentTaskRelations{BlockedBy: []string{"task-1"}, WorkspaceMode: mode})
		})
	}
}

// A blocker outside the run's mutation scope is admitted when the run task
// can read it, so a coordinator can join on children it fanned out.
//
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.2
func TestActionsCreateTaskAdmitsReadableBlocker(t *testing.T) {
	creator := relationsCreator(map[string]string{"child-a": "ws-1"})
	reads := &recordingTaskReads{allowed: map[[2]string]bool{{"task-1", "child-a"}: true}}
	actions := relationsActions(creator, reads)

	_, err := actions.CreateTask(context.Background(), relationsRunCtx(), CreateTaskInput{
		Title: "Join", BlockedBy: []string{"child-a"},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("creator calls = %d, want 1", len(creator.calls))
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.2
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.3
func TestActionsCreateTaskDeniesBlockersOutsideScopeIndistinguishably(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		taskID  string
		blocker string
	}{
		{name: "unrelated readable-scope miss", blocker: "unrelated", taskID: "task-1"},
		{name: "missing task with wildcard scope", allowed: []string{WildcardTaskScope}, blocker: "missing", taskID: "task-1"},
		{name: "foreign workspace in scope", allowed: []string{"foreign"}, blocker: "foreign", taskID: "task-1"},
		{name: "taskless run outside scope", blocker: "unrelated", taskID: ""},
	}
	messages := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			creator := relationsCreator(map[string]string{"unrelated": "ws-1", "foreign": "ws-2"})
			reads := &recordingTaskReads{}
			actions := relationsActions(creator, reads)
			runCtx := relationsRunCtx(tc.allowed...)
			runCtx.TaskID = tc.taskID

			_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
				Title: "Join", ProjectID: "project-1", BlockedBy: []string{tc.blocker},
			})
			if !errors.Is(err, ErrBlockerOutOfScope) || !errors.Is(err, shared.ErrForbidden) {
				t.Fatalf("CreateTask error = %v, want ErrBlockerOutOfScope (forbidden)", err)
			}
			if strings.Contains(err.Error(), tc.blocker) {
				t.Fatalf("error %q names the blocker ID", err)
			}
			if len(creator.calls) != 0 {
				t.Fatalf("creator called %d times after denial", len(creator.calls))
			}
			messages[err.Error()] = true
		})
	}
	if len(messages) != 1 {
		t.Fatalf("denial messages differ by cause: %v", messages)
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.4
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.6
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.7
func TestActionsCreateTaskRejectsMalformedRelationsBeforeReads(t *testing.T) {
	tooMany := make([]string, maxTaskRelationBlockers+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("dep-%d", i)
	}
	cases := []struct {
		name  string
		input CreateTaskInput
		field string
	}{
		{"empty blocker", CreateTaskInput{BlockedBy: []string{"dep-1", "  "}}, "blocked_by"},
		{"overlong blocker", CreateTaskInput{BlockedBy: []string{strings.Repeat("a", maxTaskRelationBlockerIDLength+1)}}, "blocked_by"},
		{"too many blockers", CreateTaskInput{BlockedBy: tooMany}, "blocked_by"},
		{"shared group mode", CreateTaskInput{WorkspaceMode: "shared_group"}, "workspace_mode"},
		{"unknown mode", CreateTaskInput{WorkspaceMode: "elsewhere"}, "workspace_mode"},
		{"inherit without parent", CreateTaskInput{WorkspaceMode: "inherit_parent"}, "workspace_mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			creator := relationsCreator(nil)
			reads := &recordingTaskReads{}
			actions := relationsActions(creator, reads)
			tc.input.Title = "Join"

			_, err := actions.CreateTask(context.Background(), relationsRunCtx(WildcardTaskScope), tc.input)
			if !errors.Is(err, ErrInvalidTaskRelations) {
				t.Fatalf("CreateTask error = %v, want ErrInvalidTaskRelations", err)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error %q does not name %s", err, tc.field)
			}
			if len(creator.calls) != 0 || len(creator.workspaceLookups) != 0 ||
				len(creator.projectLookups) != 0 || len(reads.asked) != 0 {
				t.Fatalf("reads or creates ran for malformed input: calls=%d ws=%v proj=%v reads=%v",
					len(creator.calls), creator.workspaceLookups, creator.projectLookups, reads.asked)
			}
		})
	}
}

func TestActionsCreateTaskSurfacesReadRelationFailure(t *testing.T) {
	creator := relationsCreator(nil)
	readErr := errors.New("relation store unavailable")
	actions := relationsActions(creator, &recordingTaskReads{err: readErr})

	_, err := actions.CreateTask(context.Background(), relationsRunCtx(), CreateTaskInput{
		Title: "Join", BlockedBy: []string{"child-a"},
	})
	if !errors.Is(err, readErr) || errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("CreateTask error = %v, want the read failure, not a denial", err)
	}
	if len(creator.calls) != 0 {
		t.Fatalf("creator called %d times after read failure", len(creator.calls))
	}
}

func TestActionsCreateTaskRequiresReadRelationOnlyWhenNeeded(t *testing.T) {
	creator := relationsCreator(map[string]string{"task-1": "ws-1"})
	actions := relationsActions(creator, nil)

	if _, err := actions.CreateTask(context.Background(), relationsRunCtx(), CreateTaskInput{
		Title: "Successor", BlockedBy: []string{"task-1"},
	}); err != nil {
		t.Fatalf("in-scope blocker without read relation: %v", err)
	}
	_, err := actions.CreateTask(context.Background(), relationsRunCtx(), CreateTaskInput{
		Title: "Join", BlockedBy: []string{"child-a"},
	})
	if !errors.Is(err, ErrRuntimeDependencyMissing) {
		t.Fatalf("out-of-scope blocker without read relation = %v, want ErrRuntimeDependencyMissing", err)
	}
}

func assertRelations(t *testing.T, got, want shared.AgentTaskRelations) {
	t.Helper()
	if got.WorkspaceMode != want.WorkspaceMode || strings.Join(got.BlockedBy, ",") != strings.Join(want.BlockedBy, ",") {
		t.Fatalf("relations = %#v, want %#v", got, want)
	}
}
