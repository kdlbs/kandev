package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestSetupChannel_CreatesChannelAndTask(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create an agent first.
	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "assistant",
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	channel := &models.Channel{
		WorkspaceID:     "ws-1",
		AgentInstanceID: agent.ID,
		Platform:        "telegram",
		Config:          `{"bot_token":"test"}`,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	if channel.ID == "" {
		t.Error("channel ID should be set")
	}
	if channel.TaskID == "" {
		t.Error("channel task_id should be set")
	}

	// Verify we can list it.
	channels, err := svc.ListChannelsByAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("got %d channels, want 1", len(channels))
	}
	if channels[0].Platform != "telegram" {
		t.Errorf("platform = %q, want telegram", channels[0].Platform)
	}
}

func TestHandleChannelInbound_CreatesComment(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "assistant",
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	channel := &models.Channel{
		WorkspaceID:     "ws-1",
		AgentInstanceID: agent.ID,
		Platform:        "telegram",
		Config:          `{"bot_token":"test"}`,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	err := svc.HandleChannelInbound(ctx, channel.ID, "user123", "Hello agent!")
	if err != nil {
		t.Fatalf("inbound: %v", err)
	}
}

func TestDeleteChannel(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "assistant",
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	channel := &models.Channel{
		WorkspaceID:     "ws-1",
		AgentInstanceID: agent.ID,
		Platform:        "slack",
		Config:          `{}`,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	if err := svc.DeleteChannel(ctx, channel.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	channels, _ := svc.ListChannelsByAgent(ctx, agent.ID)
	if len(channels) != 0 {
		t.Errorf("got %d channels after delete, want 0", len(channels))
	}
}
