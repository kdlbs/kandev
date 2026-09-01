package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestStepFromPortableNormalizesProfileSessionPolicies(t *testing.T) {
	svc, _ := setupTestService(t)

	step := svc.stepFromPortableWithMatcher("workflow-1", models.StepPortable{
		Name:                      "Review",
		Position:                  0,
		ProfileSessionStartPolicy: taskmodels.WorkflowProfileSessionStartPolicy(" unsupported "),
		ProfileSessionEndPolicy:   taskmodels.WorkflowProfileSessionEndPolicy(" unsupported "),
	}, map[int]string{0: "step-1"}, nil, "")

	require.Equal(t, taskmodels.WorkflowProfileSessionStartPolicyReuse, step.ProfileSessionStartPolicy)
	require.Equal(t, taskmodels.WorkflowProfileSessionEndPolicyComplete, step.ProfileSessionEndPolicy)
}

func TestImportWorkflowCarriesProfileSessionPoliciesOnStep(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	export := &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name: "Imported policy",
			Steps: []models.StepPortable{{
				Name:                      "Todo",
				Position:                  0,
				ProfileSessionStartPolicy: taskmodels.WorkflowProfileSessionStartPolicyReuse,
				ProfileSessionEndPolicy:   taskmodels.WorkflowProfileSessionEndPolicyPark,
			}, {
				Name:                      "Review",
				Position:                  1,
				ProfileSessionStartPolicy: taskmodels.WorkflowProfileSessionStartPolicyNew,
				ProfileSessionEndPolicy:   taskmodels.WorkflowProfileSessionEndPolicyComplete,
			}},
		}},
	}

	_, err := svc.ImportWorkflows(context.Background(), "ws-1", export)
	require.NoError(t, err)
	workflow, err := provider.GetWorkflow(context.Background(), "imported-Imported policy")
	require.NoError(t, err)
	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), workflow.ID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	require.Equal(t, taskmodels.WorkflowProfileSessionStartPolicyReuse, steps[0].ProfileSessionStartPolicy)
	require.Equal(t, taskmodels.WorkflowProfileSessionEndPolicyPark, steps[0].ProfileSessionEndPolicy)
	require.Equal(t, taskmodels.WorkflowProfileSessionStartPolicyNew, steps[1].ProfileSessionStartPolicy)
	require.Equal(t, taskmodels.WorkflowProfileSessionEndPolicyComplete, steps[1].ProfileSessionEndPolicy)
}

func TestApplySyncedWorkflowsNormalizesStepProfileSessionPolicies(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	wf := addSyncedWorkflow(provider, "workflow-sync-policy", "ws-1", "Policy", "flows/policy.yml")
	portable := portableWorkflow("Policy", "Todo")
	portable.Steps[0].ProfileSessionStartPolicy = taskmodels.WorkflowProfileSessionStartPolicyNew
	portable.Steps[0].ProfileSessionEndPolicy = taskmodels.WorkflowProfileSessionEndPolicyPark

	result, err := svc.ApplySyncedWorkflows(context.Background(), "ws-1", []SyncFileExport{{
		Path:   "flows/policy.yml",
		Export: exportOf(portable),
	}})
	require.NoError(t, err)
	require.Contains(t, result.Updated, "Policy")
	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), wf.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	require.Equal(t, taskmodels.WorkflowProfileSessionStartPolicyNew, steps[0].ProfileSessionStartPolicy)
	require.Equal(t, taskmodels.WorkflowProfileSessionEndPolicyPark, steps[0].ProfileSessionEndPolicy)

	portable.Steps[0].ProfileSessionStartPolicy = taskmodels.WorkflowProfileSessionStartPolicy("unsupported")
	portable.Steps[0].ProfileSessionEndPolicy = taskmodels.WorkflowProfileSessionEndPolicy("unsupported")
	_, err = svc.ApplySyncedWorkflows(context.Background(), "ws-1", []SyncFileExport{{
		Path:   "flows/policy.yml",
		Export: exportOf(portable),
	}})
	require.NoError(t, err)
	steps, err = svc.repo.ListStepsByWorkflow(context.Background(), wf.ID)
	require.NoError(t, err)
	require.Equal(t, taskmodels.WorkflowProfileSessionStartPolicyReuse, steps[0].ProfileSessionStartPolicy)
	require.Equal(t, taskmodels.WorkflowProfileSessionEndPolicyComplete, steps[0].ProfileSessionEndPolicy)
}
