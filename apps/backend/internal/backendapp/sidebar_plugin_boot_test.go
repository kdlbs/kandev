package backendapp

import (
	"context"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userservice "github.com/kandev/kandev/internal/user/service"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type blockedBootGrouping struct {
	entered chan struct{}
	release chan struct{}
	taskID  string
}

func (r *blockedBootGrouping) ResolveSidebarGrouping(ctx context.Context, _, reference string) (*taskmodels.SidebarPluginGrouping, error) {
	r.entered <- struct{}{}
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &taskmodels.SidebarPluginGrouping{Reference: reference, Generation: "generation", Snapshot: &pluginsdk.SidebarGroupingSnapshot{Version: 1, Complete: true, Revision: "revision", Groups: []pluginsdk.SidebarGroupingValue{{ID: "g", Label: "Group"}}, Memberships: []pluginsdk.SidebarGroupingMembership{{TaskID: r.taskID, GroupID: "g"}}}}, nil
}

func TestTaskDetailBootDoesNotWaitForPluginSidebar(t *testing.T) {
	h := newBootStateTestHarness(t)
	ctx := t.Context()
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workspaceID := workspaces[0].ID
	workflows, err := h.taskSvc.ListWorkflows(ctx, workspaceID, true)
	require.NoError(t, err)
	steps, err := h.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	require.NoError(t, err)
	created, err := h.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{WorkspaceID: workspaceID, WorkflowID: workflows[0].ID, WorkflowStepID: steps[0].ID, Title: "Cold route"})
	require.NoError(t, err)
	views := []usermodels.SidebarView{{ID: "grouped", Name: "Grouped", Group: "plugin:acme:labels", Sort: usermodels.SidebarViewSort{Key: "updatedAt", Direction: "desc"}}}
	active := "grouped"
	_, err = h.userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{WorkspaceID: &workspaceID, SidebarViews: &views, SidebarActiveViewID: &active})
	require.NoError(t, err)
	resolver := &blockedBootGrouping{entered: make(chan struct{}, 1), release: make(chan struct{}), taskID: created.Task.ID}
	h.taskSvc.SetSidebarGroupingResolver(resolver)
	builder := bootStateBuilder{p: routeParams{taskSvc: h.taskSvc, userCtrl: h.userCtrl, services: &Services{Workflow: h.workflowSvc}}}
	done := make(chan map[string]any, 1)
	go func() { done <- builder.taskDetailRouteData(ctx, nil, created.Task.ID, "") }()
	select {
	case <-resolver.entered:
		close(resolver.release)
		<-done
		t.Fatal("cold task route waited for plugin sidebar classification")
	case data := <-done:
		detail := data["taskDetail"].(map[string]any)
		require.Nil(t, detail["sidebarTaskPage"])
	}
	query, prefs := taskDetailSidebarQuery(&userdto.UserSettingsDTO{SidebarViews: views, SidebarActiveViewID: active}, workspaceID, "en")
	result := make(chan *taskmodels.SidebarTaskPageResult, 1)
	errors := make(chan error, 1)
	go func() {
		page, err := h.taskSvc.QuerySidebarTaskPage(ctx, workspaceID, query, prefs)
		result <- page
		errors <- err
	}()
	select {
	case <-resolver.entered:
	case err := <-errors:
		t.Fatalf("asynchronous sidebar query did not reach provider: %v", err)
	}
	close(resolver.release)
	page := <-result
	require.NoError(t, <-errors)
	require.Equal(t, "revision", page.GroupingRevision)
	require.Equal(t, 1, page.TotalTasks)
}
