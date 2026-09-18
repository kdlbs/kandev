package runtime

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantWorkspaceGrantDispatchRace(t *testing.T) {
	s, db, conversation := newRuntime(t)
	manager := &workspaceGrantManager{assistantTaskManager: &assistantTaskManager{}}
	s.Manager = manager
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
 INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('created-task','linked','Synthetic delivery',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	router, token, run := assistantRuntimeCaller(t, s, conversation)
	ctx := context.Background()
	b, err := s.Repo.AssistantBinding(ctx, "owner")
	require.NoError(t, err)
	receiver, err := s.workspaceGrantReceiver(ctx, b)
	require.NoError(t, err)
	g := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: receiver.ProfileID, ReceiverProfileRevision: receiver.ProfileRevision, AuthorityRevision: receiver.AuthorityRevision, Scope: models.WorkspaceGrantScope{Operations: []string{"observe", "coordinate"}, ContextExports: []string{"task_summary", "handoff"}}}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(ctx, b, g, 0))
	workerProfile, err := s.Personas.Profiles.GetAgentProfile(ctx, "personal")
	require.NoError(t, err)
	workerProfile.ID = "linked-worker"
	workerProfile.WorkspaceID = "linked"
	workerProfile.Name = "Linked worker account"
	require.NoError(t, s.Personas.Profiles.CreateAgentProfile(ctx, workerProfile))
	require.NoError(t, s.Repo.SaveAssistantMemory(ctx, &models.AgentMemory{ID: "home-only", AgentProfileID: b.OrchestratorID, OwnerUserID: b.OwnerUserID, Layer: "user", Key: "home", Scope: "workspace", ScopeID: b.WorkspaceID, Content: "SYNTHETIC_HOME_ONLY_MEMORY"}, 0))
	require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: "source", TaskID: conversation, AuthorID: "owner", AuthorType: "user", Source: "user", Body: "Create an example summary in the linked workspace."}))
	query := "?workspace_id=linked&workspace_grant_revision=1"
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/objectives"+query, token, run, map[string]any{"title": "Example summary", "mode": "execute", "source_comment_id": "source", "acceptance": []models.Criterion{{ID: "summary", Description: "Summary exists"}}, "operation_id": "linked-objective", "expected_intent_revision": 0})
	require.Equal(t, 201, response.Code, response.Body.String())
	var objective models.Objective
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &objective))
	require.Equal(t, "linked", objective.WorkspaceID)
	for _, suffix := range []string{"", "/memory"} {
		wrongTarget := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/context/"+objective.ID+suffix+"?profile_id=missing", token, run, nil)
		require.Equal(t, 403, wrongTarget.Code, "reject the unselected target before consulting its profile or context")
	}
	response = runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/context/"+objective.ID+query+"&profile_id=linked-worker", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), "SYNTHETIC_HOME_ONLY_MEMORY")
	var packet models.ContextPacket
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &packet))
	require.Equal(t, "linked", packet.WorkspaceID)
	task := &taskmodels.Task{ID: "created-task", WorkspaceID: "linked", Metadata: map[string]any{"orchestration_binding_id": b.ID, "orchestration_objective_id": objective.ID}}
	s.Tasks.(*testTasks).tasks[task.ID] = task
	require.NoError(t, s.ValidateDispatchContext(ctx, packet.ID, task, "linked-worker"))
	require.Error(t, s.ValidateDispatchContext(ctx, packet.ID, task, "personal"), "central account cannot replace the worker account")
	body := map[string]any{"title": "Example summary", "execution_mode": "execute", "objective_id": objective.ID, "context_ref": packet.ID, "operation_id": "linked-create", "expected_intent_revision": 0}
	response = runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks"+query, token, run, body)
	require.Equal(t, 201, response.Code, response.Body.String())
	require.Equal(t, "linked", manager.lastSpec.WorkspaceID)
	require.Equal(t, "linked-worker", manager.lastSpec.AssigneeID)
	response = runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, run, body)
	require.Equal(t, 409, response.Code, "same operation cannot replay a linked receipt against home")
	require.EqualValues(t, 1, manager.creates.Load())
	require.NoError(t, s.Repo.ForgetWorkspaceContext(ctx, b, "linked", 1))
	require.Error(t, s.ValidateDispatchContext(ctx, packet.ID, task, "linked-worker"), "forgetting invalidates old queued packets")
	fresh, err := s.buildContext(ctx, b, objective.ID, models.ContextScope{ProfileID: "linked-worker"})
	require.NoError(t, err)
	require.NotEqual(t, packet.ID, fresh.ID, "forgotten references cannot be recreated by fetching identical context")
	require.NoError(t, s.Repo.RevokeWorkspaceGrant(ctx, b, "linked", 2))
	require.Error(t, s.ValidateDispatchContext(ctx, packet.ID, task, "linked-worker"), "old queued context cannot dispatch after revocation")
	require.Equal(t, 403, runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks"+query, token, run, body).Code)
	require.EqualValues(t, 1, manager.creates.Load(), "completed effects are retained; revocation prevents further effects")
}
