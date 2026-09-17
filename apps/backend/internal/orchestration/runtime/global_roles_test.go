package runtime

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEachTurnUsesCurrentGlobalRoleAndWorkspaceContext(t *testing.T) {
	service, _, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, service.Repo.UpsertInstruction(ctx, "chief", "ROLE.md", "Stale copied instructions", false))
	role, err := service.Repo.GetOrchestratorRole(ctx, "chief-of-staff")
	require.NoError(t, err)
	for _, policy := range []string{"Initial role policy", "Revised role policy"} {
		role.Instructions = policy
		require.NoError(t, service.Repo.SaveOrchestratorRole(ctx, role))
		persona, err := service.Personas.GetAgentInstance(ctx, "chief")
		require.NoError(t, err)
		prompt, err := service.prompt(ctx, persona, task, nil)
		require.NoError(t, err)
		require.Contains(t, prompt, policy)
		require.NotContains(t, prompt, "Stale copied instructions")
		require.Contains(t, persona.Settings, "personal")
	}
}
