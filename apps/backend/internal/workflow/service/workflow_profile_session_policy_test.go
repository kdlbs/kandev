package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestImportWorkflowNormalizesProfileSessionPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		want   taskmodels.WorkflowProfileSessionPolicy
	}{
		{name: "park reuse", policy: "park_reuse", want: taskmodels.WorkflowProfileSessionPolicyParkReuse},
		{name: "invalid defaults to complete", policy: "unknown", want: taskmodels.WorkflowProfileSessionPolicyComplete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, provider := setupTestServiceWithProvider(t)
			export := &models.WorkflowExport{
				Version: models.ExportVersion,
				Type:    models.ExportType,
				Workflows: []models.WorkflowPortable{{
					Name:                 "Imported " + tt.name,
					ProfileSessionPolicy: taskmodels.WorkflowProfileSessionPolicy(tt.policy),
					Steps:                []models.StepPortable{{Name: "Todo", Position: 0, Color: "gray"}},
				}},
			}

			_, err := svc.ImportWorkflows(context.Background(), "ws-1", export)
			require.NoError(t, err)
			workflow, err := provider.GetWorkflow(context.Background(), "imported-Imported "+tt.name)
			require.NoError(t, err)
			require.Equal(t, tt.want, workflow.ProfileSessionPolicy)
		})
	}
}

func TestGetWorkflowMetaIncludesProfileSessionPolicy(t *testing.T) {
	svc, _ := setupTestService(t)
	provider := &mockWorkflowProvider{workflows: []*taskmodels.Workflow{{
		ID:                   "workflow-meta-policy",
		ProfileSessionPolicy: taskmodels.WorkflowProfileSessionPolicyParkReuse,
	}}}
	svc.SetWorkflowProvider(provider)

	meta, err := svc.GetWorkflowMeta(context.Background(), "workflow-meta-policy")
	require.NoError(t, err)
	field := reflect.ValueOf(meta).FieldByName("ProfileSessionPolicy")
	if !field.IsValid() {
		t.Fatal("WorkflowMeta.ProfileSessionPolicy is missing")
	}
	if got := field.Interface(); got != taskmodels.WorkflowProfileSessionPolicyParkReuse {
		t.Fatalf("profile session policy = %v, want %q", got, taskmodels.WorkflowProfileSessionPolicyParkReuse)
	}
}

func TestApplySyncedWorkflowsNormalizesProfileSessionPolicy(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	wf := addSyncedWorkflow(provider, "workflow-sync-policy", "ws-1", "Policy", "flows/policy.yml")
	portable := portableWorkflow("Policy", "Todo")
	portable.ProfileSessionPolicy = taskmodels.WorkflowProfileSessionPolicyParkNew

	result, err := svc.ApplySyncedWorkflows(context.Background(), "ws-1", []SyncFileExport{{
		Path:   "flows/policy.yml",
		Export: exportOf(portable),
	}})
	require.NoError(t, err)
	require.Contains(t, result.Updated, "Policy")
	got, err := provider.GetWorkflow(context.Background(), wf.ID)
	require.NoError(t, err)
	require.Equal(t, taskmodels.WorkflowProfileSessionPolicyParkNew, got.ProfileSessionPolicy)

	portable.ProfileSessionPolicy = "unsupported"
	_, err = svc.ApplySyncedWorkflows(context.Background(), "ws-1", []SyncFileExport{{
		Path:   "flows/policy.yml",
		Export: exportOf(portable),
	}})
	require.NoError(t, err)
	got, err = provider.GetWorkflow(context.Background(), wf.ID)
	require.NoError(t, err)
	require.Equal(t, taskmodels.WorkflowProfileSessionPolicyComplete, got.ProfileSessionPolicy)
}
