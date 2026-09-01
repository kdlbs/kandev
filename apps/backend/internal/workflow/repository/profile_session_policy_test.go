package repository

import (
	"context"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkflowStepProfileSessionPolicyRoundTripAndDefault(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	for index, policy := range []taskmodels.WorkflowProfileSessionPolicy{
		taskmodels.WorkflowProfileSessionPolicyComplete,
		taskmodels.WorkflowProfileSessionPolicyParkReuse,
		taskmodels.WorkflowProfileSessionPolicyParkNew,
	} {
		step := &models.WorkflowStep{
			WorkflowID:           "wf-test",
			Name:                 string(policy),
			Position:             index,
			ProfileSessionPolicy: policy,
		}
		if err := repo.CreateStep(ctx, step); err != nil {
			t.Fatalf("create %q: %v", policy, err)
		}
		got, err := repo.GetStep(ctx, step.ID)
		if err != nil {
			t.Fatalf("get %q: %v", policy, err)
		}
		if got.ProfileSessionPolicy != policy {
			t.Fatalf("get %q policy = %q, want %q", policy, got.ProfileSessionPolicy, policy)
		}
	}

	unknown := &models.WorkflowStep{
		WorkflowID:           "wf-test",
		Name:                 "unknown",
		Position:             99,
		ProfileSessionPolicy: taskmodels.WorkflowProfileSessionPolicy("unsupported"),
	}
	if err := repo.CreateStep(ctx, unknown); err != nil {
		t.Fatalf("create unknown: %v", err)
	}
	if unknown.ProfileSessionPolicy != taskmodels.WorkflowProfileSessionPolicyComplete {
		t.Fatalf("created step policy = %q, want normalized complete", unknown.ProfileSessionPolicy)
	}
	got, err := repo.GetStep(ctx, unknown.ID)
	if err != nil {
		t.Fatalf("get unknown: %v", err)
	}
	if got.ProfileSessionPolicy != taskmodels.WorkflowProfileSessionPolicyComplete {
		t.Fatalf("unknown policy = %q, want complete", got.ProfileSessionPolicy)
	}

	got.ProfileSessionPolicy = taskmodels.WorkflowProfileSessionPolicy(" unsupported update ")
	if err := repo.UpdateStep(ctx, got); err != nil {
		t.Fatalf("normalize updated policy: %v", err)
	}
	if got.ProfileSessionPolicy != taskmodels.WorkflowProfileSessionPolicyComplete {
		t.Fatalf("updated caller policy = %q, want normalized complete", got.ProfileSessionPolicy)
	}

	got.ProfileSessionPolicy = taskmodels.WorkflowProfileSessionPolicyParkReuse
	if err := repo.UpdateStep(ctx, got); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	got, err = repo.GetStep(ctx, unknown.ID)
	if err != nil {
		t.Fatalf("get updated policy: %v", err)
	}
	if got.ProfileSessionPolicy != taskmodels.WorkflowProfileSessionPolicyParkReuse {
		t.Fatalf("updated policy = %q, want %q", got.ProfileSessionPolicy, taskmodels.WorkflowProfileSessionPolicyParkReuse)
	}
}
