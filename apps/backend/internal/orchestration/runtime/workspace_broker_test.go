package runtime

import (
	"context"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantWorkspaceBrokerIsolation(t *testing.T) {
	s, db, conversation := newRuntime(t)
	manager := &workspaceGrantManager{assistantTaskManager: &assistantTaskManager{}}
	s.Manager = manager
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	router, token, run := assistantRuntimeCaller(t, s, conversation)
	b, err := s.Repo.AssistantBinding(context.Background(), "owner")
	require.NoError(t, err)
	path := "/api/v1/orchestration/runtime/workspace?workspace_id=linked&workspace_grant_revision=1"
	response := runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 403, response.Code, "a requested foreign target is not implicitly authorized: "+response.Body.String())
	receiver, err := s.workspaceGrantReceiver(context.Background(), b)
	require.NoError(t, err)
	g := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: receiver.ProfileID, ReceiverProfileRevision: receiver.ProfileRevision, AuthorityRevision: receiver.AuthorityRevision, Scope: models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"directory"}}}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(context.Background(), b, g, 0))
	links := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/workspace-links", token, run, nil)
	require.Equal(t, 200, links.Code, links.Body.String())
	require.Contains(t, links.Body.String(), "Example linked workspace")
	response = runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"workspace_id":"linked"`)
	manager.onCatalog = func() { require.NoError(t, s.Repo.RevokeWorkspaceGrant(context.Background(), b, "linked", 1)) }
	response = runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 403, response.Code, "a revoke while reading must prevent exporting the result")
	require.NotContains(t, response.Body.String(), "SYNTHETIC_LINKED_DIRECTORY")
	manager.onCatalog = nil
	require.Equal(t, 403, runtimeRequest(t, router, "GET", path, token, run, nil).Code)
	links = runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/workspace-links", token, run, nil)
	require.Equal(t, 200, links.Code, links.Body.String())
	require.NotContains(t, links.Body.String(), "Example linked workspace")
}
