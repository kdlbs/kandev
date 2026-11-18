package streams

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationMCPResolutionOrigins(t *testing.T) {
	rows := NormalizeMCPResolution([]MCPResolutionEntry{{RuntimeName: "reports", DefinitionID: "definition", Origins: []MCPSelectionOrigin{{Scope: "automation", WorkspaceID: "workspace", OwnerID: "automation"}}}})
	require.Len(t, rows, 1)
	require.Len(t, rows[0].Origins, 1)
	require.Equal(t, "automation", rows[0].Origins[0].Scope)
}
