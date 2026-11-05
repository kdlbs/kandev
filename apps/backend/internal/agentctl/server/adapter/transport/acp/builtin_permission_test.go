package acp

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuiltinClaudeTitlePermissionIdentity(t *testing.T) {
	for _, name := range []string{"kandev-configuration", "kandev-rich-output", "kandev-walkthroughs", "kandev-diagnostics"} {
		value := normalizeClaudePermissionToolName(nil, nil, "mcp__"+name+"__list_workspaces_kandev", "other")
		require.NotNil(t, value, name)
		require.Equal(t, "mcp__"+name+"__list_workspaces_kandev", *value)
	}
	require.Nil(t, normalizeClaudePermissionToolName(nil, nil, "mcp__kandev-configuration-extra__list_workspaces_kandev", "other"))
}
