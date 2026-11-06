package orchestrator

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/automation"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

type templateAutomationService struct{ *stubAutomationService }

func (s *templateAutomationService) AutomationMCPTaskTemplate(context.Context, string) (*mcpconfig.BuiltinCreationPreferences, []string, error) {
	revision := int64(0)
	return &mcpconfig.BuiltinCreationPreferences{Version: 1, ExpectedPolicyRevision: &revision, Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}, []string{"definition"}, nil
}
func TestAutomationMCPNormalTaskTemplate(t *testing.T) {
	repo := setupTestRepo(t)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "created"}}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.reviewTaskCreator = creator
	svc.SetAutomationService(&templateAutomationService{stubAutomationService: &stubAutomationService{}})
	a := &automation.Automation{ID: "automation", WorkspaceID: "workspace", TaskMode: automation.TaskModeNormalTask, RepositoryMode: automation.RepositoryModeNone, WorkflowID: "workflow", MCPSelection: &automation.MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, DefinitionIDs: []string{"definition"}}}
	_, _, _, _, err := svc.prepareAutomationTask(t.Context(), a, &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeManual}, "Report", "Report", map[string]interface{}{})
	require.NoError(t, err)
	raw, err := json.Marshal(creator.got)
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Contains(t, got, "BuiltinMCPPreferences")
	require.JSONEq(t, `["definition"]`, string(got["MCPServerIDs"]))
}
