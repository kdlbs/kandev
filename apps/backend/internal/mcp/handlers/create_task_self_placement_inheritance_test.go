package handlers

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/stretchr/testify/require"
)

func selfPlacementWorkflowSteps(t *testing.T, f selfPlacementFixture, workflowProfile string) {
	t.Helper()
	database := sqlx.NewDb(f.repo.DB(), "sqlite3") // The fixture owns this connection.
	steps, err := workflowrepo.NewWithDB(database, database, testLogger(t))
	require.NoError(t, err)
	wfSvc := workflowservice.NewService(steps, testLogger(t))
	t.Cleanup(func() { require.NoError(t, wfSvc.Close()) })
	f.h.workflowCtrl = workflowcontroller.NewController(wfSvc)
	getter := &staticWorkflowStepGetter{steps: map[string]*workflowmodels.WorkflowStep{}}
	for _, workflowID := range []string{f.workflow.ID, f.child.WorkflowID} {
		step := &workflowmodels.WorkflowStep{
			ID: "start-" + workflowID, WorkflowID: workflowID, Name: "Start", IsStartStep: true,
			AgentProfileID: workflowProfile,
		}
		if _, exists := getter.steps[step.ID]; exists {
			continue
		}
		require.NoError(t, steps.CreateStep(f.ctx, step))
		getter.steps[step.ID] = step
	}
	f.svc.SetWorkflowStepGetter(getter)
}

func seedSelfPlacementInheritance(t *testing.T, f selfPlacementFixture) *officesqlite.Repository {
	t.Helper()
	childWorkflow := &models.Workflow{ID: "child-workflow", WorkspaceID: f.workspace.ID, Name: "Child workflow"}
	require.NoError(t, f.repo.CreateWorkflow(f.ctx, childWorkflow))
	f.child.WorkflowID = childWorkflow.ID
	for _, task := range []*models.Task{f.parent, f.child} {
		repository := &models.Repository{ID: task.ID + "-repo", WorkspaceID: f.workspace.ID, Name: task.Title}
		require.NoError(t, f.repo.CreateRepository(f.ctx, repository))
		require.NoError(t, f.repo.CreateTaskRepository(f.ctx, &models.TaskRepository{
			TaskID: task.ID, RepositoryID: repository.ID, BaseBranch: task.ID + "-base",
		}))
		task.Metadata = map[string]interface{}{
			models.MetaKeyAgentProfileID: task.ID + "-profile", models.MetaKeyExecutorProfileID: task.ID + "-executor",
		}
		require.NoError(t, f.repo.UpdateTask(f.ctx, task))
	}
	session, err := f.repo.GetTaskSession(f.ctx, "self-placement-session")
	require.NoError(t, err)
	session.AgentProfileID, session.ExecutorProfileID = "creator-profile", "creator-executor"
	require.NoError(t, f.repo.UpdateTaskSession(f.ctx, session))
	require.NoError(t, f.repo.UpdateTaskSessionAgentProfileSnapshot(f.ctx, session.ID, map[string]interface{}{
		"model": "snapshot-model", "mode": "snapshot-mode",
	}))
	require.NoError(t, f.repo.UpdateSessionMetadata(f.ctx, session.ID, map[string]interface{}{
		models.SessionMetaKeyRuntimeConfig: models.SessionRuntimeConfig{Model: "creator-model", ConfigOptions: map[string]string{"reasoning_effort": "high"}},
		models.SessionMetaKeySessionMode:   "acceptEdits",
	}))

	database := sqlx.NewDb(f.repo.DB(), "sqlite3")
	groups, err := officesqlite.NewWithDB(database, database, testLogger(t))
	require.NoError(t, err)
	for _, task := range []*models.Task{f.parent, f.child} {
		group := &officemodels.WorkspaceGroup{ID: task.ID + "-group", WorkspaceID: f.workspace.ID,
			OwnerTaskID: task.ID, MaterializedKind: officemodels.WorkspaceGroupKindSingleRepo,
			MaterializedPath: filepath.Join(t.TempDir(), task.ID),
		}
		require.NoError(t, groups.CreateWorkspaceGroup(f.ctx, group))
		require.NoError(t, groups.AddWorkspaceGroupMember(f.ctx, group.ID, task.ID, officemodels.WorkspaceMemberRoleOwner))
	}
	f.svc.SetWorkspacePolicyAttacher(service.NewHandoffService(f.repo, nil, nil, groups, groups, testLogger(t)))
	return groups
}

// Test-only contract coverage: parent-derived scope and creator-derived runtime
// must remain distinct when self is redirected.
// @covers AC-TASKS-SELF-SIBLING-002.1 AC-TASKS-SELF-SIBLING-002.2
func TestMCPCreateTaskSelfPlacementInheritance(t *testing.T) {
	for _, name := range []string{"inherited", "explicit overrides", "workspace profile", "workflow profile"} {
		t.Run(name, func(t *testing.T) {
			f := newSelfPlacementFixture(t)
			groups := seedSelfPlacementInheritance(t, f)
			workflowProfile := ""
			if name == "workflow profile" {
				workflowProfile = "workflow-profile"
			}
			selfPlacementWorkflowSteps(t, f, workflowProfile)
			payload := selfPlacementCreatePayload(f, f.child.ID)
			delete(payload, "workspace_id")
			delete(payload, "workflow_id")
			delete(payload, "agent_profile_id")
			wantProfile, wantWorkflow := "creator-profile", f.workflow.ID
			wantRepo, wantBase := f.parent.ID+"-repo", f.parent.ID+"-base"
			wantExecutor := f.parent.ID + "-executor"
			switch name {
			case "explicit overrides":
				wantProfile, wantWorkflow, wantRepo, wantBase = "explicit-profile", f.child.WorkflowID, f.child.ID+"-repo", "override-base"
				wantExecutor = "explicit-executor"
				payload["workflow_id"], payload["workspace_mode"] = wantWorkflow, "new_workspace"
				payload["agent_profile_id"], payload["executor_profile_id"] = wantProfile, wantExecutor
				payload["repositories"] = []mcpRepositoryInput{{RepositoryID: wantRepo, BaseBranch: wantBase}}
			case "workspace profile":
				wantProfile = "workspace-profile"
				_, err := f.svc.UpdateWorkspace(f.ctx, f.workspace.ID, &service.UpdateWorkspaceRequest{DefaultAgentProfileID: &wantProfile})
				require.NoError(t, err)
				f.h.SetUserSettingsProvider(&mcpUserSettingsProvider{settings: &usermodels.UserSettings{
					MCPTaskAgentProfileDefault: usermodels.MCPTaskAgentProfileDefaultWorkspaceDefault,
				}})
			case "workflow profile":
				payload["agent_profile_id"] = "explicit-profile"
				wantProfile = workflowProfile
			}
			result := selfPlacementResult(t, f.create(t, f.caller(), payload))
			task, err := f.svc.GetTask(f.ctx, result.ID)
			require.NoError(t, err)
			require.Equal(t, f.parent.ID, task.ParentID)
			require.Equal(t, f.workspace.ID, task.WorkspaceID)
			require.Equal(t, wantWorkflow, task.WorkflowID)
			require.Len(t, task.Repositories, 1)
			require.Equal(t, wantRepo, task.Repositories[0].RepositoryID)
			require.Equal(t, wantBase, task.Repositories[0].BaseBranch)
			require.Equal(t, wantProfile, task.Metadata[models.MetaKeyAgentProfileID])
			require.Equal(t, wantExecutor, task.Metadata[models.MetaKeyExecutorProfileID])
			runtime, ok := models.LoadInitialSessionRuntimeConfig(task.Metadata)
			require.Equal(t, name == "inherited", ok)
			if ok {
				require.Equal(t, "creator-model", runtime.Model)
				require.Equal(t, "acceptEdits", runtime.Mode)
				require.Equal(t, map[string]string{"reasoning_effort": "high"}, runtime.ConfigOptions)
			}
			group, err := groups.GetWorkspaceGroupForTask(f.ctx, task.ID)
			require.NoError(t, err)
			if name == "explicit overrides" {
				require.Nil(t, group)
			} else {
				require.NotNil(t, group)
				require.Equal(t, f.parent.ID+"-group", group.ID)
				require.Equal(t, f.parent.ID, group.OwnerTaskID)
			}
			rows := ledgerRowsForTask(t, f.repo, task.ID)
			require.Len(t, rows, 1)
			require.Equal(t, string(steptelemetry.ActorAgent), rows[0].actorKind)
			require.NotNil(t, rows[0].actorID)
			require.Equal(t, "self-placement-session", *rows[0].actorID)
			require.True(t, canDirectParentAccess(f.parent, task))
			require.False(t, canDirectParentAccess(f.child, task))
		})
	}
}
