package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type importNameHTTPProvider struct{ fakeWorkflowProvider }

func (p *importNameHTTPProvider) CreateWorkflow(ctx context.Context, workspace, name, description string) (*taskmodels.Workflow, error) {
	workflow, err := p.fakeWorkflowProvider.CreateWorkflow(ctx, workspace, name, description)
	if err == nil {
		workflow.ID = uuid.NewString()
	}
	return workflow, err
}

// @covers AC-TASKS-IMPORT-PROFILES-001.7
// @covers AC-TASKS-IMPORT-PROFILES-001.10
// @covers AC-TASKS-IMPORT-PROFILES-001.11
func TestWorkflowImportNameSelectionHTTP(t *testing.T) {
	for _, form := range []string{"raw", "preview", "browser"} {
		for _, malformed := range []bool{false, true} {
			t.Run(form+map[bool]string{false: "/valid", true: "/invalid-skipped"}[malformed], func(t *testing.T) {
				h := setupStepRouter(t)
				provider := &importNameHTTPProvider{}
				h.service.SetWorkflowProvider(provider)
				skipped := models.StepPortable{Name: "Later", Position: 0, AgentProfile: &models.AgentProfilePortable{AgentName: "Unavailable"}}
				steps := []models.StepPortable{skipped}
				if malformed {
					steps = append(steps, skipped)
				}
				document := models.WorkflowExport{Version: models.LegacyExportVersion, Type: models.ExportType, Workflows: []models.WorkflowPortable{
					{Name: "Release A", Steps: []models.StepPortable{{Name: "First", Position: 0}}},
					{Name: "Release A", Steps: steps},
				}}
				body, err := yaml.Marshal(document)
				require.NoError(t, err)
				path := "/api/v1/workspaces/ws-1/workflows/import"
				if form == "preview" {
					path += "/preview"
				}
				var rec *httptest.ResponseRecorder
				if form == "browser" {
					rec = doJSON(t, h.router, http.MethodPost, path, map[string]any{"yaml": string(body), "step_profile_bindings": []any{}})
				} else {
					rec = doRaw(t, h, http.MethodPost, path, string(body))
				}
				if malformed {
					require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
					assert.Empty(t, provider.created)
					return
				}
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				if form == "preview" {
					var preview workflowservice.ImportProfilePreview
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
					assert.Equal(t, []string{"Release A"}, preview.Skipped)
					assert.Empty(t, preview.Steps)
					assert.Empty(t, provider.created)
					return
				}
				var result workflowservice.ImportResult
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				assert.Equal(t, []string{"Release A"}, result.Created)
				assert.Equal(t, []string{"Release A"}, result.Skipped)
				require.Len(t, provider.workflows, 1)
				assert.Equal(t, []string{"Release A"}, provider.created)
				persisted, err := h.repo.ListStepsByWorkflow(context.Background(), provider.workflows[0].ID)
				require.NoError(t, err)
				require.Len(t, persisted, 1)
				assert.Equal(t, "First", persisted[0].Name)
			})
		}
	}
}
