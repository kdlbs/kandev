package runtime

import (
	"context"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantWorkspaceGrantAttentionRestart(t *testing.T) {
	s, db, b, f := assistantAttentionFixture(t)
	ctx := context.Background()
	s.Manager = &workspaceGrantManager{assistantTaskManager: &assistantTaskManager{}}
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
 UPDATE tasks SET workspace_id='linked' WHERE id='worker'; UPDATE orchestration_objectives SET workspace_id='linked' WHERE binding_id=?`, b.ID)
	require.NoError(t, err)
	s.Tasks.(*testTasks).tasks["worker"].WorkspaceID = "linked"
	for i := range f.sources {
		f.sources[i].Summary = "SYNTHETIC_RESTRICTED_REQUEST_TEXT"
	}
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	require.Zero(t, f.reads, "no grant means no native observation")
	r, err := s.workspaceGrantReceiver(ctx, b)
	require.NoError(t, err)
	g := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: r.ProfileID, ReceiverProfileRevision: r.ProfileRevision, AuthorityRevision: r.AuthorityRevision, Scope: models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"task_summary"}}}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(ctx, b, g, 0))
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	require.Equal(t, 1, f.reads)
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, "linked", row.WorkspaceID)
		require.NotContains(t, row.Summary, "SYNTHETIC_RESTRICTED")
	}
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.Contains(t, run.Payload, "workspace_grant_revision")
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, assistantBrokerAudience, run.Payload, "late-session"))
	session := &taskmodels.TaskSession{ID: "late-session", TaskID: b.ConversationID, Metadata: map[string]any{AssistantPolicyMetadata: string(mcpprofile.SurfaceAssistantBroker)}}
	_, err = db.Exec(`UPDATE orchestration_attention SET state='resolved' WHERE binding_id=?`, b.ID)
	require.NoError(t, err)
	require.NoError(t, s.CheckAssistantSession(ctx, b.ConversationID, session, "personal"), "resolving the wake's request must allow its active session to acknowledge the result")
	require.NoError(t, s.Repo.RevokeWorkspaceGrant(ctx, b, "linked", 1))
	require.Error(t, s.CheckAssistantSession(ctx, b.ConversationID, session, "personal"), "native final dispatch must recheck the queued wake grant")
	s.Start = func(context.Context, Launch) error {
		t.Fatal("revoked queued wake must not reach the provider")
		return nil
	}
	_, err = s.Process(ctx, run)
	require.Error(t, err)
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	require.Equal(t, 1, f.reads)
	response := runtimeRequest(t, assistantRouter(s), "GET", "/api/v1/orchestration/assistant/attention", "", "", nil)
	require.Equal(t, 200, response.Code)
	require.NotContains(t, response.Body.String(), "worker")
}

func TestAssistantWorkspaceGrantInputScope(t *testing.T) {
	s, db, b, _ := assistantAttentionFixture(t)
	ctx := context.Background()
	s.Manager = &workspaceGrantManager{assistantTaskManager: &assistantTaskManager{}}
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
 UPDATE tasks SET workspace_id='linked' WHERE id='worker'; UPDATE orchestration_objectives SET workspace_id='linked' WHERE binding_id=?`, b.ID)
	require.NoError(t, err)
	s.Tasks.(*testTasks).tasks["worker"].WorkspaceID = "linked"
	r, err := s.workspaceGrantReceiver(ctx, b)
	require.NoError(t, err)
	g := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: r.ProfileID, ReceiverProfileRevision: r.ProfileRevision, AuthorityRevision: r.AuthorityRevision, Scope: models.WorkspaceGrantScope{Operations: []string{"observe", "coordinate"}, ContextExports: []string{"task_summary", "handoff"}}}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(ctx, b, g, 0))
	router, token, run := maintenanceRuntimeCaller(t, s, b)
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	var row models.Attention
	for _, candidate := range rows {
		if candidate.Kind == "question" {
			row = candidate
		}
	}
	s.Inputs = &testAssistantInputs{input: models.AttentionInput{SourceID: row.SourceID, Kind: row.Kind, State: row.State, SourceRevision: row.SourceRevision, TaskID: row.TaskID, SessionID: row.SessionID, Questions: []models.InputQuestion{{ID: "color", Prompt: "SYNTHETIC_GRANTED_QUESTION"}}}}
	path := "/api/v1/orchestration/runtime/attention/" + row.ID + "/input"
	query := "?workspace_id=linked&workspace_grant_revision="
	require.Equal(t, 403, runtimeRequest(t, router, "GET", path+query+"1", token, run, nil).Code)
	require.Equal(t, 404, runtimeRequest(t, router, "GET", path, token, run, nil).Code, "a foreign attention ID is not an implicit target")
	g.Scope.ContextExports = append(g.Scope.ContextExports, "task_input")
	require.NoError(t, s.Repo.SaveWorkspaceGrant(ctx, b, g, 1))
	response := runtimeRequest(t, router, "GET", path+query+"2", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "SYNTHETIC_GRANTED_QUESTION")
	require.NoError(t, s.Repo.RevokeWorkspaceGrant(ctx, b, "linked", 2))
	response = runtimeRequest(t, router, "GET", path+query+"2", token, run, nil)
	require.Equal(t, 403, response.Code)
	require.NotContains(t, response.Body.String(), "SYNTHETIC_GRANTED_QUESTION")
}
