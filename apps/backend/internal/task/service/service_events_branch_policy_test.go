package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskEventsSerializeBranchPolicySnapshot(t *testing.T) {
	createdAt := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)

	for _, eventType := range []string{events.TaskCreated, events.TaskUpdated} {
		t.Run(eventType, func(t *testing.T) {
			svc, eventBus, _ := createTestService(t)
			task := &models.Task{
				ID: "task-policy-event", WorkspaceID: "ws-policy-event", WorkflowID: "workflow-policy-event",
				Title: "Policy event", CreatedAt: createdAt, UpdatedAt: updatedAt,
				Repositories: []*models.TaskRepository{{
					ID: "task-repo-policy-event", TaskID: "task-policy-event", RepositoryID: "repo-policy-event",
					BaseBranch: "main", CheckoutBranch: "feature/policy-event", Position: 0,
					BranchPolicyID: "policy-event", BranchPolicyName: "Feature branches",
					BranchPolicyBaseBranch: "main", BranchPolicyBranchTemplate: "feature/{title}-{suffix}",
					BranchPolicyPullRequestTarget: "develop", Metadata: map[string]interface{}{"source": "test"},
					CreatedAt: createdAt, UpdatedAt: updatedAt,
				}},
			}

			svc.publishTaskEvent(context.Background(), eventType, task, nil)

			published := eventBus.GetPublishedEvents()
			if len(published) != 1 || published[0].Type != eventType {
				t.Fatalf("published events = %#v, want one %s event", published, eventType)
			}
			data, ok := published[0].Data.(map[string]interface{})
			if !ok {
				t.Fatalf("event data type = %T, want map[string]interface{}", published[0].Data)
			}
			repositories, ok := data["repositories"].([]map[string]interface{})
			if !ok || len(repositories) != 1 {
				t.Fatalf("repositories = %#v, want one serialized repository", data["repositories"])
			}
			repository := repositories[0]
			for key, want := range map[string]string{
				"branch_policy_id":                  "policy-event",
				"branch_policy_name":                "Feature branches",
				"branch_policy_base_branch":         "main",
				"branch_policy_branch_template":     "feature/{title}-{suffix}",
				"branch_policy_pull_request_target": "develop",
			} {
				if got := repository[key]; got != want {
					t.Errorf("repositories[0][%q] = %#v, want %q", key, got, want)
				}
			}
			if got := repository["metadata"]; got == nil {
				t.Error("repositories[0][metadata] is missing")
			}
			if got := repository["created_at"]; got != createdAt.Format(time.RFC3339) {
				t.Errorf("repositories[0][created_at] = %#v, want %q", got, createdAt.Format(time.RFC3339))
			}
			if got := repository["updated_at"]; got != updatedAt.Format(time.RFC3339) {
				t.Errorf("repositories[0][updated_at] = %#v, want %q", got, updatedAt.Format(time.RFC3339))
			}
		})
	}
}
