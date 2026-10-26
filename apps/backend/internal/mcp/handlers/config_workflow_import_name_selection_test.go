package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowsvc "github.com/kandev/kandev/internal/workflow/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type importNameMCPProvider struct{ memWorkflowProvider }

func (p *importNameMCPProvider) CreateWorkflow(ctx context.Context, workspace, name, description string) (*taskmodels.Workflow, error) {
	workflow, err := p.memWorkflowProvider.CreateWorkflow(ctx, workspace, name, description)
	if err == nil {
		workflow.ID = uuid.NewString()
	}
	return workflow, err
}

// @covers AC-TASKS-IMPORT-PROFILES-001.10
// @covers AC-TASKS-IMPORT-PROFILES-001.11
func TestHandleImportWorkflow_DocumentNameSelection(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "invalid-skipped"}[malformed], func(t *testing.T) {
			h, _, repo := setupImportHandlers(t)
			provider := &importNameMCPProvider{}
			h.workflowSvc.SetWorkflowProvider(provider)
			later := wfmodels.StepPortable{Name: "Later", Position: 0}
			laterSteps := []wfmodels.StepPortable{later}
			if malformed {
				laterSteps = append(laterSteps, later)
			}
			document, err := yaml.Marshal(wfmodels.WorkflowExport{Version: wfmodels.LegacyExportVersion, Type: wfmodels.ExportType, Workflows: []wfmodels.WorkflowPortable{
				{Name: "Release A", Steps: []wfmodels.StepPortable{{Name: "First", Position: 0}}},
				{Name: "Release A", Steps: laterSteps},
			}})
			require.NoError(t, err)
			response, err := h.handleImportWorkflow(context.Background(), makeWSMessage(t, ws.ActionMCPImportWorkflow, map[string]string{"workspace_id": "ws-1", "document": string(document)}))
			require.NoError(t, err)
			if malformed {
				assert.Equal(t, ws.MessageTypeError, response.Type)
				assert.Contains(t, string(response.Payload), "duplicate step position")
				assert.Empty(t, provider.workflows)
				return
			}
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			var result workflowsvc.ImportResult
			require.NoError(t, json.Unmarshal(response.Payload, &result))
			assert.Equal(t, []string{"Release A"}, result.Created)
			assert.Equal(t, []string{"Release A"}, result.Skipped)
			require.Len(t, provider.workflows, 1)
			steps, err := repo.ListStepsByWorkflow(context.Background(), provider.workflows[0].ID)
			require.NoError(t, err)
			require.Len(t, steps, 1)
			assert.Equal(t, "First", steps[0].Name)
		})
	}
}
