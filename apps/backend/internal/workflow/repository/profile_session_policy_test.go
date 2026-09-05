package repository

import (
	"context"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkflowStepProfileSessionPoliciesRoundTripAndDefaults(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	for index, policies := range []struct {
		start taskmodels.WorkflowProfileSessionStartPolicy
		end   taskmodels.WorkflowProfileSessionEndPolicy
	}{
		{taskmodels.WorkflowProfileSessionStartPolicyReuse, taskmodels.WorkflowProfileSessionEndPolicyComplete},
		{taskmodels.WorkflowProfileSessionStartPolicyNew, taskmodels.WorkflowProfileSessionEndPolicyComplete},
		{taskmodels.WorkflowProfileSessionStartPolicyReuse, taskmodels.WorkflowProfileSessionEndPolicyPark},
		{taskmodels.WorkflowProfileSessionStartPolicyNew, taskmodels.WorkflowProfileSessionEndPolicyPark},
	} {
		step := &models.WorkflowStep{
			WorkflowID:                "wf-test",
			Name:                      string(policies.start) + "-" + string(policies.end),
			Position:                  index,
			ProfileSessionStartPolicy: policies.start,
			ProfileSessionEndPolicy:   policies.end,
		}
		if err := repo.CreateStep(ctx, step); err != nil {
			t.Fatalf("create %q/%q: %v", policies.start, policies.end, err)
		}
		got, err := repo.GetStep(ctx, step.ID)
		if err != nil {
			t.Fatalf("get %q/%q: %v", policies.start, policies.end, err)
		}
		if got.ProfileSessionStartPolicy != policies.start || got.ProfileSessionEndPolicy != policies.end {
			t.Fatalf("get %q/%q policies = %q/%q, want %q/%q", policies.start, policies.end, got.ProfileSessionStartPolicy, got.ProfileSessionEndPolicy, policies.start, policies.end)
		}
	}

	unknown := &models.WorkflowStep{
		WorkflowID:                "wf-test",
		Name:                      "unknown",
		Position:                  99,
		ProfileSessionStartPolicy: taskmodels.WorkflowProfileSessionStartPolicy("unsupported"),
		ProfileSessionEndPolicy:   taskmodels.WorkflowProfileSessionEndPolicy("unsupported"),
	}
	if err := repo.CreateStep(ctx, unknown); err != nil {
		t.Fatalf("create unknown: %v", err)
	}
	if unknown.ProfileSessionStartPolicy != taskmodels.WorkflowProfileSessionStartPolicyReuse || unknown.ProfileSessionEndPolicy != taskmodels.WorkflowProfileSessionEndPolicyComplete {
		t.Fatalf("created policies = %q/%q, want normalized reuse/complete", unknown.ProfileSessionStartPolicy, unknown.ProfileSessionEndPolicy)
	}
	got, err := repo.GetStep(ctx, unknown.ID)
	if err != nil {
		t.Fatalf("get unknown: %v", err)
	}
	if got.ProfileSessionStartPolicy != taskmodels.WorkflowProfileSessionStartPolicyReuse || got.ProfileSessionEndPolicy != taskmodels.WorkflowProfileSessionEndPolicyComplete {
		t.Fatalf("unknown policies = %q/%q, want reuse/complete", got.ProfileSessionStartPolicy, got.ProfileSessionEndPolicy)
	}

	got.ProfileSessionStartPolicy = taskmodels.WorkflowProfileSessionStartPolicy(" unsupported update ")
	got.ProfileSessionEndPolicy = taskmodels.WorkflowProfileSessionEndPolicy(" unsupported update ")
	if err := repo.UpdateStep(ctx, got); err != nil {
		t.Fatalf("normalize updated policies: %v", err)
	}
	if got.ProfileSessionStartPolicy != taskmodels.WorkflowProfileSessionStartPolicyReuse || got.ProfileSessionEndPolicy != taskmodels.WorkflowProfileSessionEndPolicyComplete {
		t.Fatalf("updated caller policies = %q/%q, want reuse/complete", got.ProfileSessionStartPolicy, got.ProfileSessionEndPolicy)
	}

	got.ProfileSessionStartPolicy = taskmodels.WorkflowProfileSessionStartPolicyNew
	got.ProfileSessionEndPolicy = taskmodels.WorkflowProfileSessionEndPolicyPark
	if err := repo.UpdateStep(ctx, got); err != nil {
		t.Fatalf("update policies: %v", err)
	}
	got, err = repo.GetStep(ctx, unknown.ID)
	if err != nil {
		t.Fatalf("get updated policy: %v", err)
	}
	if got.ProfileSessionStartPolicy != taskmodels.WorkflowProfileSessionStartPolicyNew || got.ProfileSessionEndPolicy != taskmodels.WorkflowProfileSessionEndPolicyPark {
		t.Fatalf("updated policies = %q/%q, want new/park", got.ProfileSessionStartPolicy, got.ProfileSessionEndPolicy)
	}
}
