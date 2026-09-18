package models

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleMaintenanceScope() MaintenanceScope {
	return MaintenanceScope{RepositoryID: "repository", WorkflowID: "workflow", WorkflowStepID: "review", ProfileID: "profile",
		Files: []string{"scripts/format-sample.js", "tests/format-sample.test.js"}, Actions: []string{"read", "patch", "test", "commit"},
		Image:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Positive: []string{"node", "--test", "tests/positive.js"}, Negative: []string{"node", "--test", "tests/negative.js"}}
}

// @covers AC-ORCHESTRATION-ASSISTANT-008.3
func TestAssistantMaintenanceBoundary(t *testing.T) {
	valid := sampleMaintenanceScope()
	require.NoError(t, valid.Validate())
	for _, file := range []string{"../outside", "/absolute", "scripts/../outside", "**", ".git/config", ".github/workflows/publish.yml", ".claude/settings.json", "AGENTS.md", "apps/backend/internal/auth/policy.go", "apps/backend/internal/agentctl/approval.go", "settings/permissions.json", "src/allowlist.ts"} {
		t.Run(file, func(t *testing.T) {
			scope := valid
			scope.Files = []string{file}
			require.Error(t, scope.Validate())
		})
	}
	for _, action := range []string{"push", "pr", "deploy", "restart", "approve", "shell", "policy"} {
		scope := valid
		scope.Actions = append(slices.Clone(valid.Actions), action)
		require.Error(t, scope.Validate(), action)
	}
	invalid := valid
	invalid.Negative = nil
	require.Error(t, invalid.Validate(), "negative validation is mandatory")
	invalid = valid
	invalid.Files = append(slices.Clone(valid.Files), valid.Files[0])
	require.Error(t, invalid.Validate(), "ambiguous duplicate scope")
}
