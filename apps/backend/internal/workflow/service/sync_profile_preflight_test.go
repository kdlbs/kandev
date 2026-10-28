package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PORTABLE-PROFILE-001.2
func TestSyncAmbiguousProfilePreservesWorkflow(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	wf.Description = "before"
	createStep(t, svc, &models.WorkflowStep{ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, Color: "#aabbcc", AgentProfileID: "old"})
	pw := portableWorkflow("Dev Flow", "Todo")
	pw.Description = "after"
	pw.Steps[0].AgentProfile = &models.AgentProfilePortable{AgentName: "Codex", Model: "luna"}
	svc.SetAgentProfileLookup(func(*models.AgentProfilePortable, string) models.ProfileMatch {
		return models.ProfileMatch{ID: "wrong", Reason: "multiple matching profiles"}
	})
	result, err := svc.ApplySyncedWorkflows(context.Background(), "ws-1", []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}})
	require.NoError(t, err)
	require.NotEmpty(t, result.Warnings)
	require.Empty(t, result.Updated)
	require.Equal(t, "before", wf.Description)
	steps, err := svc.ListStepsByWorkflow(context.Background(), wf.ID)
	require.NoError(t, err)
	require.Equal(t, "old", steps[0].AgentProfileID)
}

// @covers AC-TASKS-PORTABLE-PROFILE-001.2
func TestSyncProfileFailureSkipsOnlyAffectedWorkflowAndRecovers(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Blocked", "flows/dev.yml")
	wf.Description = "before"
	createStep(t, svc, &models.WorkflowStep{ID: "old-step", WorkflowID: wf.ID, Name: "Todo", Position: 0, Color: "#aabbcc", AgentProfileID: "old"})
	blocked := portableWorkflow("Blocked", "Todo")
	blocked.Description = "after"
	blocked.Steps[0].AgentProfile = &models.AgentProfilePortable{AgentName: "Codex", Model: "luna"}
	other := portableWorkflow("Other", "Todo")
	lookupFailed := true
	svc.SetAgentProfileLookup(func(*models.AgentProfilePortable, string) models.ProfileMatch {
		if lookupFailed {
			return models.ProfileMatch{Reason: "profile lookup failed"}
		}
		return models.ProfileMatch{ID: "new"}
	})
	files := []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(blocked, other)}}
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1)
	require.Contains(t, result.Warnings[0], "flows/dev.yml")
	require.Contains(t, result.Warnings[0], "Blocked")
	require.Equal(t, []string{"Other"}, result.Created)
	require.Equal(t, "before", wf.Description)
	lookupFailed = false
	result, err = svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	require.Contains(t, result.Updated, "Blocked")
	steps, err := svc.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Equal(t, "new", steps[0].AgentProfileID)
}

// @covers AC-TASKS-PORTABLE-PROFILE-001.1
func TestSyncUsesResolvedDefaultAndReviewProfiles(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Flow", "flows/dev.yml")
	createStep(t, svc, &models.WorkflowStep{ID: "old-step", WorkflowID: wf.ID, Name: "Review", Position: 0, Color: "#aabbcc"})
	pw := portableWorkflow("Flow", "Review")
	low := map[string]string{"reasoning_effort": "low"}
	high := map[string]string{"reasoning_effort": "high"}
	pw.AgentProfile = &models.AgentProfilePortable{AgentName: "Codex", ConfigOptions: &low}
	pw.Steps[0].Events = models.StepEvents{OnEnter: []models.OnEnterAction{{
		Type:   models.OnEnterRunCodeReview,
		Config: map[string]interface{}{models.ReviewAgentProfilePortableKey: map[string]interface{}{"agent_name": "Codex", "config_options": high}},
	}}}
	svc.SetAgentProfileLookup(func(profile *models.AgentProfilePortable, _ string) models.ProfileMatch {
		if profile.ConfigOptions != nil && (*profile.ConfigOptions)["reasoning_effort"] == "low" {
			return models.ProfileMatch{ID: "low"}
		}
		return models.ProfileMatch{ID: "high"}
	})
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}})
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	require.Equal(t, "low", wf.AgentProfileID)
	steps, err := svc.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Equal(t, "high", steps[0].Events.OnEnter[0].Config[models.ReviewAgentProfileConfigKey])
}

// @covers AC-TASKS-PORTABLE-PROFILE-001.6
func TestSyncReviewProfileReadFailurePreservesBinding(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Flow", "flows/dev.yml")
	wf.Description = "before"
	createStep(t, svc, &models.WorkflowStep{ID: "old-step", WorkflowID: wf.ID, Name: "Review", Position: 0, Color: "#aabbcc", Events: models.StepEvents{OnEnter: []models.OnEnterAction{{Type: models.OnEnterRunCodeReview, Config: map[string]interface{}{models.ReviewAgentProfileConfigKey: "existing"}}}}})
	pw := portableWorkflow("Flow", "Review")
	pw.Description = "after"
	pw.Steps[0].Events = models.StepEvents{OnEnter: []models.OnEnterAction{{Type: models.OnEnterRunCodeReview, Config: map[string]interface{}{models.ReviewAgentProfilePortableKey: map[string]interface{}{"agent_name": "Codex"}}}}}
	svc.SetAgentProfileLookup(func(*models.AgentProfilePortable, string) models.ProfileMatch {
		return models.ProfileMatch{Reason: "profile lookup failed"}
	})
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}})
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "before", wf.Description)
	steps, err := svc.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Equal(t, "existing", steps[0].Events.OnEnter[0].Config[models.ReviewAgentProfileConfigKey])
}
