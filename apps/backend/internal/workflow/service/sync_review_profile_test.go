package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestApplySyncedWorkflows_PreservesCurrentReviewAgentProfileBinding(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	wf.Description = "synced Dev Flow"
	createStep(t, svc, &models.WorkflowStep{
		ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, Color: "#aabbcc", IsStartStep: true,
		Events: models.StepEvents{OnEnter: []models.OnEnterAction{{
			Type:   models.OnEnterRunCodeReview,
			Config: map[string]interface{}{models.ReviewAgentProfileConfigKey: "profile-current"},
		}}},
	})

	pw := portableWorkflow("Dev Flow", "Todo")
	pw.Steps[0].Events = models.StepEvents{OnEnter: []models.OnEnterAction{{
		Type: models.OnEnterRunCodeReview,
		Config: map[string]interface{}{models.ReviewAgentProfilePortableKey: map[string]interface{}{
			"agent_name": "Claude", "model": "opus[1m]", "mode": "auto",
		}},
	}}}
	svc.SetAgentProfileFuncs(func(id string) *models.AgentProfilePortable {
		if id == "profile-current" {
			return &models.AgentProfilePortable{AgentName: "Claude", Model: "opus[1m]", Mode: "auto"}
		}
		return nil
	}, func(string, string, string, string) string { return "profile-oldest" })

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{{
		Path: "flows/dev.yml", Export: exportOf(pw),
	}})
	require.NoError(t, err)
	assert.Empty(t, result.Updated, "an existing exact review binding must win over a global candidate")

	steps, err := svc.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	require.Len(t, steps[0].Events.OnEnter, 1)
	assert.Equal(t, "profile-current", steps[0].Events.OnEnter[0].Config[models.ReviewAgentProfileConfigKey])
}

func TestApplySyncedWorkflows_LogsReviewAgentProfileRebinding(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	svc, provider, _ := setupSyncServiceWithLogger(t, log)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	wf.Description = "synced Dev Flow"
	createStep(t, svc, &models.WorkflowStep{
		ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, Color: "#aabbcc", IsStartStep: true,
		Events: models.StepEvents{OnEnter: []models.OnEnterAction{{
			Type:   models.OnEnterRunCodeReview,
			Config: map[string]interface{}{models.ReviewAgentProfileConfigKey: "profile-old"},
		}}},
	})

	pw := portableWorkflow("Dev Flow", "Todo")
	pw.Steps[0].Events = models.StepEvents{OnEnter: []models.OnEnterAction{{
		Type: models.OnEnterRunCodeReview,
		Config: map[string]interface{}{models.ReviewAgentProfilePortableKey: map[string]interface{}{
			"agent_name": "Claude", "model": "opus[1m]", "mode": "auto",
		}},
	}}}
	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "profile-new" })

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{{
		Path: "flows/dev.yml", Export: exportOf(pw),
	}})
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Updated)

	entries := logs.FilterMessage("workflow sync rebinding step's review agent profile").All()
	require.Len(t, entries, 1)
	assert.Equal(t, "profile-old", entries[0].ContextMap()["old_agent_profile_id"])
	assert.Equal(t, "profile-new", entries[0].ContextMap()["new_agent_profile_id"])
}

func TestFindReviewProfileRebindingsIgnoresActionOrder(t *testing.T) {
	existing := models.StepEvents{OnEnter: []models.OnEnterAction{
		{Type: models.OnEnterRunCodeReview, Config: map[string]interface{}{models.ReviewAgentProfileConfigKey: "profile-a"}},
		{Type: models.OnEnterRunCodeReview, Config: map[string]interface{}{models.ReviewAgentProfileConfigKey: "profile-b"}},
	}}
	desired := models.StepEvents{OnEnter: []models.OnEnterAction{
		{Type: models.OnEnterRunCodeReview, Config: map[string]interface{}{models.ReviewAgentProfileConfigKey: "profile-b"}},
		{Type: models.OnEnterRunCodeReview, Config: map[string]interface{}{models.ReviewAgentProfileConfigKey: "profile-a"}},
	}}

	assert.Empty(t, findReviewProfileRebindings(existing, desired))
}
