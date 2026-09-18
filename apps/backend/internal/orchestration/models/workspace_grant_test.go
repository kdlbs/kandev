package models

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantWorkspaceGrantScope(t *testing.T) {
	require.NoError(t, (WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"task_summary"}}).Validate())
	for _, scope := range []WorkspaceGrantScope{
		{},
		{Operations: []string{"observe", "admin"}, ContextExports: []string{"task_summary"}},
		{Operations: []string{"observe"}, ContextExports: []string{"all_transcripts"}},
		{Operations: []string{"observe", "observe"}, ContextExports: []string{"task_summary"}},
		{Operations: []string{"coordinate"}, ContextExports: []string{"handoff"}},
	} {
		require.Error(t, scope.Validate())
	}
}
