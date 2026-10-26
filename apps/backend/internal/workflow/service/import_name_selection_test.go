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

// @covers AC-TASKS-IMPORT-PROFILES-001.7
// @covers AC-TASKS-IMPORT-PROFILES-001.10
func TestWorkflowImportDocumentDuplicates(t *testing.T) {
	for _, mode := range []string{"raw", "bound"} {
		for _, version := range []int{models.LegacyExportVersion, models.ExportVersion} {
			t.Run(fmt.Sprintf("%s/v%d", mode, version), func(t *testing.T) {
				f := newImportNameFixture(t)
				result, err := f.submit(mode, importNameDocument(version, importNameWorkflow("Release A", "First lane"), importNameWorkflow("Release A", "Later lane")), nil)
				require.NoError(t, err)
				assert.Equal(t, []string{"Release A"}, result.Created)
				assert.Equal(t, []string{"Release A"}, result.Skipped)
				workflows := f.workflows(t)
				require.Len(t, workflows, 1)
				assert.Equal(t, "First lane", workflows[0].Description)
				steps, err := f.svc.repo.ListStepsByWorkflow(context.Background(), workflows[0].ID)
				require.NoError(t, err)
				require.Len(t, steps, 1)
				assert.Equal(t, "First lane", steps[0].Name)
				assert.Equal(t, []string{"workflows:INSERT:Release A", "workflow_steps:INSERT:First lane"}, f.audit(t))
			})
		}
	}
}

func TestWorkflowImportMixedNameSelection(t *testing.T) {
	for _, mode := range []string{"raw", "bound"} {
		t.Run(mode, func(t *testing.T) {
			f := newImportNameFixture(t)
			visible := f.seed(t, "ws-1", "Existing", false)
			hidden := f.seed(t, "ws-1", "Hidden", true)
			foreign := f.seed(t, "ws-other", "Foreign", false)
			names := []string{"Existing", "Release A", "Release A", "Existing", "Release B", "Hidden", "Release B", "release a", " Release A ", "Foreign"}
			var portable []models.WorkflowPortable
			for i, name := range names {
				portable = append(portable, importNameWorkflow(name, fmt.Sprintf("lane-%d", i)))
			}
			f.clearAudit(t)
			result, err := f.submit(mode, importNameDocument(models.ExportVersion, portable...), nil)
			require.NoError(t, err)
			assert.Equal(t, []string{"Release A", "Release B", "release a", " Release A ", "Foreign"}, result.Created)
			assert.Equal(t, []string{"Existing", "Release A", "Existing", "Hidden", "Release B"}, result.Skipped)
			require.Len(t, f.workflows(t), 7)
			require.Len(t, f.audit(t), 10, "exactly five workflow and five lane inserts")
			for _, id := range []string{visible.ID, hidden.ID, foreign.ID} {
				stored, err := f.tasks.GetWorkflow(context.Background(), id)
				require.NoError(t, err)
				assert.Equal(t, "existing content", stored.Description)
				steps, err := f.svc.repo.ListStepsByWorkflow(context.Background(), id)
				require.NoError(t, err)
				require.Len(t, steps, 1)
				assert.Equal(t, "Existing lane", steps[0].Name)
			}
		})
	}
}

// @covers AC-TASKS-IMPORT-PROFILES-001.2
func TestWorkflowImportPreviewSelection(t *testing.T) {
	f := newImportNameFixture(t)
	f.seed(t, "ws-1", "Existing", false)
	requested := &models.AgentProfilePortable{AgentName: "Missing"}
	first := importNameWorkflow("First", "No profile")
	duplicate := importNameWorkflow("First", "Unneeded profile")
	duplicate.Steps[0].AgentProfile = requested
	f.clearAudit(t)
	preview, err := f.svc.PreviewImportWorkflows(context.Background(), "ws-1", importNameDocument(models.ExportVersion, first, duplicate))
	require.NoError(t, err, "a skipped-only descriptor needs no profile catalog")
	assert.Empty(t, preview.Steps)
	assert.Equal(t, []string{"First"}, preview.Skipped)
	assert.Empty(t, f.audit(t))
	profile := ImportProfileCandidate{ID: "chosen", AgentName: "Missing", UpdatedAt: time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)}
	f.svc.SetImportProfileCatalog(&importProfileCatalogFixture{profiles: []ImportProfileCandidate{profile}})
	f.svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return profile.ID })
	selected := importNameWorkflow("Selected", "Profile lane")
	selected.Steps[0].AgentProfile = requested
	preview, err = f.svc.PreviewImportWorkflows(context.Background(), "ws-1", importNameDocument(models.ExportVersion,
		importNameWorkflow("Existing", "Skipped"), first, duplicate, selected, selected))
	require.NoError(t, err)
	assert.Equal(t, []string{"Existing", "First", "Selected"}, preview.Skipped)
	require.Len(t, preview.Steps, 1)
	assert.Equal(t, 3, preview.Steps[0].WorkflowIndex)
	assert.Equal(t, 0, preview.Steps[0].StepPosition)
	require.NotNil(t, preview.Steps[0].MatchedProfile)
	assert.Equal(t, profile.ID, preview.Steps[0].MatchedProfile.ID)
	assert.Empty(t, f.audit(t))
}

// @covers AC-TASKS-IMPORT-PROFILES-001.4
// @covers AC-TASKS-IMPORT-PROFILES-001.5
func TestWorkflowImportBindingSelection(t *testing.T) {
	for _, skippedBinding := range []bool{false, true} {
		t.Run(fmt.Sprintf("skipped-binding-%t", skippedBinding), func(t *testing.T) {
			f := newImportNameFixture(t)
			f.seed(t, "ws-1", "Existing", false)
			profile := ImportProfileCandidate{ID: "selected-id", UpdatedAt: time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)}
			f.svc.SetImportProfileCatalog(&importProfileCatalogFixture{profiles: []ImportProfileCandidate{profile}})
			requested := &models.AgentProfilePortable{AgentName: "Unavailable descriptor"}
			selected := importNameWorkflow("Selected", "Implement")
			selected.Steps[0].AgentProfile = requested
			position := 0
			selected.Steps = append(selected.Steps, models.StepPortable{Name: "Review", Position: 1, SessionTarget: &models.WorkflowSessionTargetPortable{Kind: models.WorkflowSessionTargetStep, StepPosition: &position}})
			duplicate := importNameWorkflow("Selected", "Skipped missing profile")
			duplicate.Steps[0].AgentProfile = requested
			other := importNameWorkflow("Other", "Other profile")
			other.Steps[0].AgentProfile = requested
			document := importNameDocument(models.ExportVersion, importNameWorkflow("Existing", "Skipped"), selected, duplicate, other)
			bindings := []ImportProfileBinding{
				{WorkflowIndex: 1, StepPosition: 0, RequestedProfile: requested, ProfileID: profile.ID, ProfileUpdatedAt: profile.UpdatedAt},
				{WorkflowIndex: 3, StepPosition: 0, RequestedProfile: requested, ProfileID: profile.ID, ProfileUpdatedAt: profile.UpdatedAt},
			}
			if skippedBinding {
				bindings = append(bindings, ImportProfileBinding{WorkflowIndex: 2, StepPosition: 0, RequestedProfile: requested, ProfileID: "removed"})
			}
			f.clearAudit(t)
			result, err := f.submit("bound", document, bindings)
			require.NoError(t, err)
			assert.Equal(t, []string{"Selected", "Other"}, result.Created)
			assert.Equal(t, []string{"Existing", "Selected"}, result.Skipped)
			for _, workflow := range f.workflows(t) {
				if workflow.Name == "Existing" {
					continue
				}
				steps, err := f.svc.repo.ListStepsByWorkflow(context.Background(), workflow.ID)
				require.NoError(t, err)
				assert.Equal(t, profile.ID, steps[0].AgentProfileID)
				if workflow.Name == "Selected" {
					require.Len(t, steps, 2)
					require.NotNil(t, steps[1].SessionTarget)
					assert.Equal(t, steps[0].ID, steps[1].SessionTarget.StepID)
				}
			}
			assert.NotContains(t, f.audit(t), "workflow_steps:INSERT:Skipped missing profile")
		})
	}
	t.Run("retained-missing-binding-is-not-replaced-by-later-binding", func(t *testing.T) {
		f := newImportNameFixture(t)
		workflow := importNameWorkflow("Selected", "Build")
		workflow.Steps[0].AgentProfile = &models.AgentProfilePortable{AgentName: "Missing"}
		_, err := f.submit("bound", importNameDocument(models.ExportVersion, workflow, workflow), []ImportProfileBinding{{WorkflowIndex: 1, StepPosition: 0, RequestedProfile: workflow.Steps[0].AgentProfile, ProfileID: "ignored"}})
		var conflict *ImportProfileResolutionError
		require.ErrorAs(t, err, &conflict)
		require.NotEmpty(t, conflict.Conflicts)
		assert.Equal(t, 0, conflict.Conflicts[0].WorkflowIndex)
		assert.Empty(t, f.audit(t))
	})
}

// @covers AC-TASKS-IMPORT-PROFILES-001.11
func TestWorkflowImportSelectionValidation(t *testing.T) {
	for _, mode := range []string{"raw", "bound", "preview"} {
		t.Run(mode, func(t *testing.T) {
			f := newImportNameFixture(t)
			malformed := importNameWorkflow("Release A", "Invalid later entry")
			malformed.Steps = append(malformed.Steps, malformed.Steps[0])
			document := importNameDocument(models.ExportVersion, importNameWorkflow("Release A", "First"), malformed)
			var err error
			if mode == "preview" {
				_, err = f.svc.PreviewImportWorkflows(context.Background(), "ws-1", document)
			} else {
				_, err = f.submit(mode, document, nil)
			}
			require.ErrorContains(t, err, "duplicate step position")
			assert.Empty(t, f.audit(t))
		})
	}
	for _, malformed := range []string{"unknown-key", "duplicate-key", "descriptor-mismatch"} {
		t.Run(malformed, func(t *testing.T) {
			f := newImportNameFixture(t)
			first := importNameWorkflow("Release A", "First")
			skipped := importNameWorkflow("Release A", "Skipped profile")
			requested := &models.AgentProfilePortable{AgentName: "Missing"}
			skipped.Steps[0].AgentProfile = requested
			binding := ImportProfileBinding{WorkflowIndex: 1, StepPosition: 0, RequestedProfile: requested, ProfileID: "unavailable"}
			bindings := []ImportProfileBinding{binding}
			switch malformed {
			case "unknown-key":
				bindings[0].StepPosition = 99
			case "duplicate-key":
				bindings = append(bindings, binding)
			case "descriptor-mismatch":
				bindings[0].RequestedProfile = &models.AgentProfilePortable{AgentName: "Wrong"}
			}
			_, err := f.submit("bound", importNameDocument(models.ExportVersion, first, skipped), bindings)
			var invalid *InvalidImportProfileBindingsError
			require.ErrorAs(t, err, &invalid)
			assert.Empty(t, f.audit(t))
		})
	}
}

func TestWorkflowImportSelectionRechecksDestination(t *testing.T) {
	f := newImportNameFixture(t)
	document := importNameDocument(models.ExportVersion, importNameWorkflow("Arrives later", "Import lane"), importNameWorkflow("Still new", "New lane"))
	preview, err := f.svc.PreviewImportWorkflows(context.Background(), "ws-1", document)
	require.NoError(t, err)
	assert.Empty(t, preview.Skipped)
	existing := f.seed(t, "ws-1", "Arrives later", false)
	f.clearAudit(t)
	result, err := f.submit("bound", document, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Still new"}, result.Created)
	assert.Equal(t, []string{"Arrives later"}, result.Skipped)
	stored, err := f.tasks.GetWorkflow(context.Background(), existing.ID)
	require.NoError(t, err)
	assert.Equal(t, "existing content", stored.Description)
	assert.Equal(t, []string{"workflows:INSERT:Still new", "workflow_steps:INSERT:New lane"}, f.audit(t))
}
