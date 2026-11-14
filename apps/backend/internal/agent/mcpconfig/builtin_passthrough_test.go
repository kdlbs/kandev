package mcpconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinCodexApprovalRequiresHostProvenance(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		server := types.McpServer{Name: "kandev-configuration", Type: "http", URL: "http://localhost:1234/mcp/packages/configuration"}
		if trusted {
			server.BuiltinPackage = p.PackageConfiguration
		}
		args, err := codexServerArgs(server)
		require.NoError(t, err)
		require.Equal(t, trusted, strings.Contains(strings.Join(args, " "), "default_tools_approval_mode"))
		encoded, err := json.Marshal(server)
		require.NoError(t, err)
		var decoded types.McpServer
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.False(t, decoded.IsInjectedBuiltin(), "wire frames cannot forge host provenance")
	}
}
