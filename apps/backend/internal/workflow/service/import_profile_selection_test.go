package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

type importProfileCatalogFixture struct {
	profiles []ImportProfileCandidate
}

func (f *importProfileCatalogFixture) ListEligibleProfiles(context.Context) ([]ImportProfileCandidate, error) {
	return append([]ImportProfileCandidate(nil), f.profiles...), nil
}

func (f *importProfileCatalogFixture) GetEligibleProfile(_ context.Context, id string) (*ImportProfileCandidate, error) {
	for _, profile := range f.profiles {
		if profile.ID == id {
			copy := profile
			return &copy, nil
		}
	}
	return nil, ErrImportProfileNotFound
}

func TestPreviewImportWorkflowsReportsMatchedAndMissingDirectProfiles(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	provider.addWorkflow("existing", "ws-1", "Already there")

	matchedAt := time.Date(2026, 9, 17, 13, 0, 0, 0, time.UTC)
	svc.SetImportProfileCatalog(&importProfileCatalogFixture{profiles: []ImportProfileCandidate{
		{ID: "matched", Name: "Codex Default", AgentName: "Codex", Model: "gpt-5", Mode: "full", UpdatedAt: matchedAt},
		{ID: "replacement", Name: "Claude Fallback", AgentName: "Claude", Model: "sonnet", Mode: "full", UpdatedAt: matchedAt},
	}})
	svc.SetAgentProfileFuncs(nil, func(agentName, model, mode, _ string) string {
		if agentName == "Codex" && model == "gpt-5" && mode == "full" {
			return "matched"
		}
		return ""
	})

	preview, err := svc.PreviewImportWorkflows(context.Background(), "ws-1", &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{
			{Name: "Already there", Steps: []models.StepPortable{{Name: "Skipped", Position: 0}}},
			{Name: "Needs profiles", Steps: []models.StepPortable{
				{Name: "Build", Position: 0, AgentProfile: &models.AgentProfilePortable{AgentName: "Codex", Model: "gpt-5", Mode: "full"}},
				{Name: "Review", Position: 1, AgentProfile: &models.AgentProfilePortable{AgentName: "Unknown", Model: "model", Mode: "mode"}},
			}},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"Already there"}, preview.Skipped)
	require.Len(t, preview.Steps, 2)
	assert.Equal(t, 1, preview.Steps[0].WorkflowIndex)
	assert.Equal(t, "Build", preview.Steps[0].StepName)
	require.NotNil(t, preview.Steps[0].MatchedProfile)
	assert.Equal(t, "matched", preview.Steps[0].MatchedProfile.ID)
	assert.Equal(t, matchedAt, preview.Steps[0].MatchedProfile.UpdatedAt)
	assert.Nil(t, preview.Steps[1].MatchedProfile)
	assert.Len(t, preview.Profiles, 2)
	assert.Len(t, provider.workflows, 1, "preview must not create workflows")
}

func TestImportWorkflowsWithBindingsUsesExactIndependentSelections(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	updatedAt := time.Date(2026, 9, 17, 13, 0, 0, 0, time.UTC)
	svc.SetImportProfileCatalog(&importProfileCatalogFixture{profiles: []ImportProfileCandidate{
		{ID: "profile-a", Name: "A", AgentName: "Agent A", Model: "model-a", Mode: "mode-a", UpdatedAt: updatedAt},
		{ID: "profile-b", Name: "B", AgentName: "Agent B", Model: "model-b", Mode: "mode-b", UpdatedAt: updatedAt},
	}})
	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string {
		t.Fatal("interactive import rematched a directly selected profile")
		return ""
	})

	requested := &models.AgentProfilePortable{AgentName: "Missing", Model: "missing", Mode: "mode"}
	export := &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name: "Selected profiles",
			Steps: []models.StepPortable{
				{Name: "First", Position: 0, AgentProfile: requested},
				{Name: "Second", Position: 1, AgentProfile: requested},
			},
		}},
	}

	result, err := svc.ImportWorkflowsWithBindings(context.Background(), "ws-1", export, []ImportProfileBinding{
		{WorkflowIndex: 0, StepPosition: 0, RequestedProfile: requested, ProfileID: "profile-a", ProfileUpdatedAt: updatedAt},
		{WorkflowIndex: 0, StepPosition: 1, RequestedProfile: requested, ProfileID: "profile-b", ProfileUpdatedAt: updatedAt},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"Selected profiles"}, result.Created)
	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), "imported-Selected profiles")
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, "profile-a", steps[0].AgentProfileID)
	assert.Equal(t, "profile-b", steps[1].AgentProfileID)
	assert.Len(t, provider.workflows, 1)
}

func TestImportWorkflowsWithBindingsRejectsRevisionChangeBeforeAnyWrite(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	svc.SetImportProfileCatalog(&importProfileCatalogFixture{profiles: []ImportProfileCandidate{
		{ID: "profile-a", Name: "A", AgentName: "Agent A", Model: "model-a", Mode: "mode-a", UpdatedAt: time.Date(2026, 9, 17, 14, 0, 0, 0, time.UTC)},
	}})
	requested := &models.AgentProfilePortable{AgentName: "Missing", Model: "missing", Mode: "mode"}
	export := &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name:  "Changed profile",
			Steps: []models.StepPortable{{Name: "Build", Position: 0, AgentProfile: requested}},
		}},
	}

	_, err := svc.ImportWorkflowsWithBindings(context.Background(), "ws-1", export, []ImportProfileBinding{
		{WorkflowIndex: 0, StepPosition: 0, RequestedProfile: requested, ProfileID: "profile-a", ProfileUpdatedAt: time.Date(2026, 9, 17, 13, 0, 0, 0, time.UTC)},
	})

	var conflict *ImportProfileResolutionError
	require.ErrorAs(t, err, &conflict)
	require.Len(t, conflict.Conflicts, 1)
	assert.Equal(t, ImportProfileReasonChanged, conflict.Conflicts[0].Reason)
	assert.Empty(t, provider.workflows, "revision conflicts must not create any workflow")
}

func TestImportWorkflowsWithBindingsRejectsMissingBindingWithoutWrites(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	svc.SetImportProfileCatalog(&importProfileCatalogFixture{})
	export := &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name:  "Missing selection",
			Steps: []models.StepPortable{{Name: "Build", Position: 0, AgentProfile: &models.AgentProfilePortable{AgentName: "Missing"}}},
		}},
	}

	_, err := svc.ImportWorkflowsWithBindings(context.Background(), "ws-1", export, nil)
	var conflict *ImportProfileResolutionError
	require.ErrorAs(t, err, &conflict)
	require.Len(t, conflict.Conflicts, 1)
	assert.Equal(t, ImportProfileReasonMissingSelection, conflict.Conflicts[0].Reason)
	assert.Empty(t, provider.workflows)
}

func TestImportWorkflowsWithBindingsCleansUpWorkflowWhenPersistenceFails(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	provider.forceUpdateWorkflowErr = fmt.Errorf("update workflow failed")
	export := &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name:   "Failed persistence",
			Prompt: "Imported prompt",
			Steps:  []models.StepPortable{{Name: "Build", Position: 0}},
		}},
	}

	_, err := svc.ImportWorkflowsWithBindings(context.Background(), "ws-1", export, nil)
	require.Error(t, err)
	assert.Empty(t, provider.workflows, "a failed import must not leave the workflow available for retries")
	assert.Equal(t, []string{"imported-Failed persistence"}, provider.deleted)
}

func TestImportWorkflowsWithBindingsCleansUpPreviouslyCreatedSteps(t *testing.T) {
	svc, db, provider := setupTestServiceWithProvider(t)
	_, err := db.Exec(`
		CREATE TRIGGER fail_import_step
		BEFORE INSERT ON workflow_steps
		WHEN NEW.name = 'Fail'
	BEGIN
		SELECT RAISE(ABORT, 'forced step failure');
	END`)
	require.NoError(t, err)
	export := &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name: "Failed step persistence",
			Steps: []models.StepPortable{
				{Name: "Create", Position: 0},
				{Name: "Fail", Position: 1},
			},
		}},
	}

	_, err = svc.ImportWorkflowsWithBindings(context.Background(), "ws-1", export, nil)
	require.Error(t, err)
	assert.Empty(t, provider.workflows)
	assert.Equal(t, []string{"imported-Failed step persistence"}, provider.deleted)
	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), "imported-Failed step persistence")
	require.NoError(t, err)
	assert.Empty(t, steps, "a failed import must remove steps created before the failing step")
}

func TestValidateImportProfileBindingsRejectsMalformedBindingSets(t *testing.T) {
	requested := &models.AgentProfilePortable{AgentName: "Agent", Model: "model", Mode: "mode"}
	export := &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name:  "Imported",
			Steps: []models.StepPortable{{Name: "Build", Position: 0, AgentProfile: requested}},
		}},
	}
	expected := expectedImportProfileSteps(export)

	tests := []struct {
		name     string
		bindings []ImportProfileBinding
	}{
		{
			name: "unknown step key",
			bindings: []ImportProfileBinding{{
				WorkflowIndex: 0, StepPosition: 99, RequestedProfile: requested, ProfileID: "profile",
			}},
		},
		{
			name: "duplicate binding",
			bindings: []ImportProfileBinding{
				{WorkflowIndex: 0, StepPosition: 0, RequestedProfile: requested, ProfileID: "profile"},
				{WorkflowIndex: 0, StepPosition: 0, RequestedProfile: requested, ProfileID: "profile"},
			},
		},
		{
			name: "descriptor mismatch",
			bindings: []ImportProfileBinding{{
				WorkflowIndex: 0, StepPosition: 0,
				RequestedProfile: &models.AgentProfilePortable{AgentName: "Other"},
				ProfileID:        "profile",
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateImportProfileBindings(test.bindings, expected)
			var bindingsErr *InvalidImportProfileBindingsError
			require.ErrorAs(t, err, &bindingsErr)
		})
	}
}
