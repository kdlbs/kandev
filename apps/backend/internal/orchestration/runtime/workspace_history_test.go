package runtime

import (
	"context"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantWorkspaceHistoryReconfirmation(t *testing.T) {
	s, db, conversation := newRuntime(t)
	s.Manager = &workspaceGrantManager{assistantTaskManager: &assistantTaskManager{}}
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, _, _ = assistantRuntimeCaller(t, s, conversation)
	ctx := context.Background()
	b, err := s.Repo.AssistantBinding(ctx, "owner")
	require.NoError(t, err)
	r, err := s.workspaceGrantReceiver(ctx, b)
	require.NoError(t, err)
	g := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: r.ProfileID, ReceiverProfileRevision: r.ProfileRevision, AuthorityRevision: r.AuthorityRevision, Scope: models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"task_summary"}}}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(ctx, b, g, 0))
	require.NoError(t, s.Repo.RecordWorkspaceExport(ctx, b, g, "task_summary"))
	require.NoError(t, s.Repo.RevokeWorkspaceGrant(ctx, b, "linked", 1))
	_, err = s.assistantAuthority(ctx, conversation)
	require.NoError(t, err, "revocation does not promise to erase previously delivered history")
	p, err := s.Personas.Profiles.GetAgentProfile(ctx, "personal")
	require.NoError(t, err)
	p.Name = "Different example account"
	require.NoError(t, s.Personas.Profiles.UpdateAgentProfile(ctx, p))
	_, err = s.assistantAuthority(ctx, conversation)
	require.ErrorContains(t, err, "history requires workspace reconfirmation")
	r, err = s.workspaceGrantReceiver(ctx, b)
	require.NoError(t, err)
	g.ReceiverProfileRevision = r.ProfileRevision
	g.Scope.ContextExports = []string{"directory"}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(ctx, b, g, 2))
	_, err = s.assistantAuthority(ctx, conversation)
	require.Error(t, err, "a narrower grant cannot authorize existing task summary history")
	g.Scope.ContextExports = []string{"task_summary"}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(ctx, b, g, 3))
	_, err = s.assistantAuthority(ctx, conversation)
	require.NoError(t, err)
}
