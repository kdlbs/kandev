package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

type fakeExclusionAdvisor struct {
	gotWorkspace string
	gotDecls     []models.AuthorExclusionDeclaration
	warnings     []ImportWarning
	err          error
	calls        int
}

func (f *fakeExclusionAdvisor) AdviseExclusionSetup(
	_ context.Context, workspaceID string, decls []models.AuthorExclusionDeclaration,
) ([]ImportWarning, error) {
	f.calls++
	f.gotWorkspace = workspaceID
	f.gotDecls = decls
	return f.warnings, f.err
}

func exclusionExport(names ...string) *models.WorkflowExport {
	export := &models.WorkflowExport{Version: models.ExportVersion, Type: models.ExportType}
	for _, name := range names {
		export.Workflows = append(export.Workflows, models.WorkflowPortable{Name: name, Steps: []models.StepPortable{
			{Name: "Work", Position: 0},
			{Name: "Review", Position: 1, Events: models.StepEvents{OnEnter: []models.OnEnterAction{
				{Type: models.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{
					"role": "reviewer", models.ExcludeAuthorProviderConfigKey: true,
				}},
			}}},
		}})
	}
	return export
}

var routingOffWarning = ImportWarning{Code: "routing_disabled", WorkflowName: "Guarded", StepName: "Review", Role: "reviewer"}

func TestPreviewImportWorkflowsReportsExclusionWarnings(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	provider.addWorkflow("existing", "ws-1", "Already there")
	advisor := &fakeExclusionAdvisor{warnings: []ImportWarning{routingOffWarning}}
	svc.SetImportExclusionAdvisor(advisor)

	preview, err := svc.PreviewImportWorkflows(context.Background(), "ws-1", exclusionExport("Already there", "Guarded"))

	require.NoError(t, err)
	assert.Equal(t, []ImportWarning{routingOffWarning}, preview.Warnings)
	assert.Equal(t, "ws-1", advisor.gotWorkspace)
	assert.Equal(t, []models.AuthorExclusionDeclaration{
		{WorkflowName: "Guarded", StepName: "Review", Role: "reviewer"},
	}, advisor.gotDecls, "skipped workflows are not imported, so they are not advised on")
}

func TestImportWorkflowsReportsExclusionWarningsAndStillImports(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	svc.SetImportExclusionAdvisor(&fakeExclusionAdvisor{warnings: []ImportWarning{routingOffWarning}})

	result, err := svc.ImportWorkflows(context.Background(), "ws-1", exclusionExport("Guarded"))

	require.NoError(t, err)
	assert.Equal(t, []string{"Guarded"}, result.Created)
	assert.Equal(t, []ImportWarning{routingOffWarning}, result.Warnings)
}

func TestImportExclusionAdvisorFailureIsCheckFailedWarning(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	svc.SetImportExclusionAdvisor(&fakeExclusionAdvisor{err: errors.New("db down")})

	result, err := svc.ImportWorkflows(context.Background(), "ws-1", exclusionExport("Guarded"))

	require.NoError(t, err)
	assert.Equal(t, []string{"Guarded"}, result.Created)
	assert.Equal(t, []ImportWarning{{Code: ImportWarningCheckFailed}}, result.Warnings)
}

func TestImportWithoutDeclarationsDoesNotCallAdvisor(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	advisor := &fakeExclusionAdvisor{}
	svc.SetImportExclusionAdvisor(advisor)
	export := &models.WorkflowExport{Version: models.ExportVersion, Type: models.ExportType,
		Workflows: []models.WorkflowPortable{{Name: "Plain", Steps: []models.StepPortable{{Name: "Todo", Position: 0}}}}}

	result, err := svc.ImportWorkflows(context.Background(), "ws-1", export)

	require.NoError(t, err)
	assert.Empty(t, result.Warnings)
	assert.Zero(t, advisor.calls)
}

func TestApplySyncedWorkflowsAppendsExclusionWarnings(t *testing.T) {
	svc, _, _ := setupSyncService(t)
	svc.SetImportExclusionAdvisor(&fakeExclusionAdvisor{warnings: []ImportWarning{
		routingOffWarning,
		{Code: "agent_single_provider", WorkflowName: "Guarded", StepName: "Review", Role: "reviewer",
			ProviderID: "codex-acp", AgentName: "QA"},
	}})

	result, err := svc.ApplySyncedWorkflows(context.Background(), "ws-1", []SyncFileExport{
		{Path: "flows/guarded.yml", Export: exclusionExport("Guarded")},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"Guarded"}, result.Created)
	assert.Equal(t, []string{
		`Guarded/Review: reviewer author-provider exclusion is not enforced because provider routing is off`,
		`Guarded/Review: reviewer author-provider exclusion can hold reviews: agent "QA" can only use codex-acp, so its review runs wait whenever the work ran on codex-acp`,
	}, result.Warnings)
}
