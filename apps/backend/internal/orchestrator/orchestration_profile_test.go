package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"testing"
)

func TestDelegatedTaskRetainsAccountProfile(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "delegated", Title: "Review", State: "CREATED", Metadata: map[string]interface{}{"orchestration_execution_profile_id": "work-profile"}}); err != nil {
		t.Fatal(err)
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	if got := svc.resolveEffectiveAgentProfile(ctx, "delegated", "", "personal-profile"); got != "work-profile" {
		t.Fatalf("delegation must retain work account, got %q", got)
	}
}
