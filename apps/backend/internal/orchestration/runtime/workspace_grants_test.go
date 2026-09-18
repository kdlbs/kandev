package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
	"testing"
)

type workspaceGrantManager struct {
	*assistantTaskManager
	denied    bool
	onAccess  func()
	onCatalog func()
}

func (m *workspaceGrantManager) AssistantWorkspaceDirectory(_ context.Context, workspace string) ([]models.WorkspaceDirectoryEntry, error) {
	if m.onCatalog != nil {
		m.onCatalog()
	}
	return []models.WorkspaceDirectoryEntry{{ID: "example", Kind: "workflow", Name: "SYNTHETIC_LINKED_DIRECTORY"}}, nil
}
func (m *workspaceGrantManager) AssistantWorkspaceTask(context.Context, string, string, bool) (models.WorkspaceTaskView, error) {
	return models.WorkspaceTaskView{}, nil
}
func (m *workspaceGrantManager) AssistantWorkspaceTasks(context.Context, string, int, int) ([]models.WorkspaceTaskSummary, bool, error) {
	return []models.WorkspaceTaskSummary{}, false, nil
}

func (m *workspaceGrantManager) WorkspaceGrantOptions(context.Context) ([]models.WorkspaceGrantOption, error) {
	return []models.WorkspaceGrantOption{{ID: "linked", Name: "Example linked workspace"}}, nil
}
func (m *workspaceGrantManager) WorkspaceGrantAccess(ctx context.Context, id string, _ bool) (models.WorkspaceGrantOption, error) {
	owner, ok := authn.IdentityFromContext(ctx)
	if m.denied || !ok || owner.UserID != "owner" || id != "linked" {
		return models.WorkspaceGrantOption{}, fmt.Errorf("native access denied")
	}
	if m.onAccess != nil {
		m.onAccess()
	}
	return models.WorkspaceGrantOption{ID: id, Name: "Example linked workspace"}, nil
}

func TestAssistantWorkspaceGrantRechecksAfterNativeAccess(t *testing.T) {
	s, db, _ := newRuntime(t)
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	router := assistantRouter(s)
	require.Equal(t, 200, runtimeRequest(t, router, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	b, err := s.Repo.AssistantBinding(context.Background(), "owner")
	require.NoError(t, err)
	receiver, err := s.workspaceGrantReceiver(context.Background(), b)
	require.NoError(t, err)
	g := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: receiver.ProfileID, ReceiverProfileRevision: receiver.ProfileRevision, AuthorityRevision: receiver.AuthorityRevision, Scope: models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"task_summary"}}}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(context.Background(), b, g, 0))
	ready, release := make(chan struct{}), make(chan struct{})
	s.Manager = &workspaceGrantManager{assistantTaskManager: &assistantTaskManager{}, onAccess: func() { close(ready); <-release }}
	result := make(chan error, 1)
	go func() {
		_, err := s.currentWorkspaceGrant(context.Background(), b, "linked", 1, "observe", "task_summary")
		result <- err
	}()
	<-ready
	require.NoError(t, s.Repo.RevokeWorkspaceGrant(context.Background(), b, "linked", 1))
	close(release)
	require.Error(t, <-result, "revocation during native validation prevents returning stale authority")
}

func TestAssistantWorkspaceGrantHumanControl(t *testing.T) {
	s, db, _ := newRuntime(t)
	s.Manager = &workspaceGrantManager{assistantTaskManager: &assistantTaskManager{}}
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	router := assistantRouter(s)
	require.Equal(t, 200, runtimeRequest(t, router, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	b, err := s.Repo.AssistantBinding(context.Background(), "owner")
	require.NoError(t, err)
	base := "/api/v1/orchestration/assistant/workspace-links"
	response := runtimeRequest(t, router, "GET", "/api/v1/orchestration/assistant/workspace-options", "", "", nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var options struct {
		Receiver models.WorkspaceGrantReceiver `json:"receiver"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &options))
	require.Equal(t, "personal", options.Receiver.ProfileID)
	body := map[string]any{"expected_binding_version": b.Version, "expected_revision": 0, "receiver": options.Receiver, "scope": models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"task_summary"}}}
	response = runtimeRequest(t, router, "PUT", base+"/linked", "", "", body)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, 409, runtimeRequest(t, router, "PUT", base+"/linked", "", "", body).Code)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", base, "", "", nil).Code)
	runtimeRouter, token, run := maintenanceRuntimeCaller(t, s, b)
	require.Equal(t, 403, runtimeRequest(t, runtimeRouter, "PUT", base+"/linked", token, run, body).Code)
	response = runtimeRequest(t, router, "GET", base, "", "", nil)
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"active":true`)
	profile, err := s.Personas.Profiles.GetAgentProfile(context.Background(), "personal")
	require.NoError(t, err)
	profile.Name = "Changed example profile"
	require.NoError(t, s.Personas.Profiles.UpdateAgentProfile(context.Background(), profile))
	response = runtimeRequest(t, router, "GET", base, "", "", nil)
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"active":false`)
	body["expected_revision"] = 1
	require.Equal(t, 409, runtimeRequest(t, router, "PUT", base+"/linked", "", "", body).Code, "an old form cannot confirm a new receiving account")
	response = runtimeRequest(t, router, "DELETE", base+"/linked", "", "", body)
	require.Equal(t, 200, response.Code, response.Body.String())
	grant, err := s.Repo.WorkspaceGrant(context.Background(), b.ID, "linked")
	require.NoError(t, err)
	require.NotNil(t, grant.RevokedAt)
	response = runtimeRequest(t, router, "GET", base+"/linked/events", "", "", nil)
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"action":"revoked"`)
}
