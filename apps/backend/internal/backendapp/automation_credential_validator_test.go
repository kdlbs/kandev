package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/automation"
	"github.com/stretchr/testify/require"
)

func TestAutomationCredentialRepositorySelectionUsesCurrentDefinition(t *testing.T) {
	selected := []string{"repo-current"}
	require.True(t, automationCredentialRepositorySelected(selected, "repo-current"))
	require.False(t, automationCredentialRepositorySelected(selected, "repo-removed"))
	require.False(t, automationCredentialRepositorySelected(selected, ""))

	choices := []automation.CredentialChoiceProjection{
		{Kind: automation.CredentialChoiceWorkspace, RepositoryID: "repo-current", Key: "SERVICE_KEY"},
		{Kind: automation.CredentialChoiceWorkspace, RepositoryID: "repo-removed", Key: "SERVICE_KEY"},
		{Kind: automation.CredentialChoicePersonal, ProfileID: "agent", Key: "SERVICE_KEY"},
	}
	validated := automationCredentialChoicesForRepositories(choices, selected)
	require.Len(t, validated, 2)
	require.Equal(t, "repo-current", validated[0].RepositoryID)
	require.Equal(t, automation.CredentialChoicePersonal, validated[1].Kind)
}
