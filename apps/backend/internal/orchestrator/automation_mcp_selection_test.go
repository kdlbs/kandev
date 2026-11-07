package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/automation"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

type templateAutomationService struct{ *stubAutomationService }

func (s *templateAutomationService) AutomationMCPTaskTemplate(context.Context, string) (*mcpconfig.BuiltinCreationPreferences, []string, []mcpconfig.MCPResolutionDecision, error) {
	revision := int64(0)
	return &mcpconfig.BuiltinCreationPreferences{Version: 1, ExpectedPolicyRevision: &revision, Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}, []string{"definition"}, []mcpconfig.MCPResolutionDecision{{DefinitionID: "missing", ReasonCode: "definition_missing"}}, nil
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
	require.NotNil(t, creator.got.BuiltinMCPPreferences)
	require.Contains(t, creator.got.Metadata, mcpconfig.TemplateUnavailableMetadataKey)
	require.Equal(t, mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}, creator.got.BuiltinMCPPreferences.Packages)
	require.EqualValues(t, 0, *creator.got.BuiltinMCPPreferences.ExpectedPolicyRevision)
	require.Equal(t, []string{"definition"}, creator.got.MCPServerIDs)
	a.MCPSelection = nil // Retry snapshots omit the mutable selection.
	_, _, _, _, err = svc.prepareAutomationTask(t.Context(), a, &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeManual}, "Retry", "Retry", map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, creator.got.BuiltinMCPPreferences)
	require.Equal(t, 1, creator.got.BuiltinMCPPreferences.Version)
	require.Equal(t, mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}, creator.got.BuiltinMCPPreferences.Packages)
	require.EqualValues(t, 0, *creator.got.BuiltinMCPPreferences.ExpectedPolicyRevision)
	require.Equal(t, []string{"definition"}, creator.got.MCPServerIDs)
}
