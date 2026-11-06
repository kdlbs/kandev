package automation

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationMCPUsageIgnoresPackageEligibility(t *testing.T) {
	svc := newTestService(t)
	a, err := svc.CreateAutomation(t.Context(), &CreateAutomationRequest{WorkspaceID: "workspace", Name: "Report"})
	require.NoError(t, err)
	_, err = svc.store.db.Exec(`UPDATE automations SET mcp_selection = ? WHERE id = ?`, `{"builtin_packages":["retired"],"definition_ids":["one","two","one"]}`, a.ID)
	require.NoError(t, err)
	count, err := svc.CountMCPDefinitionSelections(t.Context(), "workspace", "one")
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
