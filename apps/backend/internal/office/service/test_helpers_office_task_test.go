package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// createOfficeTask creates a task row and sets the assignee. Originally
// lived in execution_policy_test.go (now removed); kept as a shared
// test helper because several event-subscriber tests build a task with
// an assignee through the same channel-setup path.
func createOfficeTask(t *testing.T, svc *service.Service, wsID, assigneeID string) string {
	t.Helper()
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: wsID,
		Name:        "ch-" + t.Name(),
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create channel agent: %v", err)
	}

	channel := &models.Channel{
		WorkspaceID:    wsID,
		AgentProfileID: agent.ID,
		Platform:       "webhook",
		Config:         `{}`,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	if err := svc.SetTaskAssignee(ctx, channel.TaskID, assigneeID); err != nil {
		t.Fatalf("set assignee: %v", err)
	}
	return channel.TaskID
}
