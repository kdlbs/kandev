package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-MCP-WORKSPACE-MODE-004.3
func TestResolveMCPWorkspacePolicyCompatibility(t *testing.T) {
	h := &Handlers{}
	for _, tc := range []struct {
		name, parent, mode, want, wantError string
	}{
		{name: "root omission"},
		{name: "parent omission", parent: "parent", want: "inherit_parent"},
		{name: "root blank", mode: " \t"},
		{name: "parent blank", parent: "parent", mode: " \t", want: "inherit_parent"},
		{name: "inherit", parent: "parent", mode: "inherit_parent", want: "inherit_parent"},
		{name: "inherit needs parent", mode: "inherit_parent", wantError: "requires parent_id"},
		{name: "new root", mode: "new_workspace", want: "new_workspace"},
		{name: "new child", parent: "parent", mode: "new_workspace", want: "new_workspace"},
		// Direct backend policy handling retains trimming; MCP schema validation
		// rejects padded input before it reaches this resolver.
		{name: "trim inheritance", parent: "parent", mode: " inherit_parent ", want: "inherit_parent"},
		{name: "trim new", mode: " new_workspace ", want: "new_workspace"},
		{name: "shared", mode: "shared", wantError: "invalid workspace_mode"},
		{name: "shared group", mode: "shared_group", wantError: "invalid workspace_mode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy, err := h.resolveMCPWorkspacePolicy(tc.parent, tc.mode)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, policy.Mode)
		})
	}
}
