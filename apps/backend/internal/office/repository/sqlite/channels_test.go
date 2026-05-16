package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestChannel_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	channel := &models.Channel{
		WorkspaceID:    "ws-1",
		AgentProfileID: "agent-1",
		Platform:       "telegram",
		Config:         `{"bot_token":"xxx"}`,
		Status:         "active",
	}
	if err := repo.CreateChannel(ctx, channel); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetChannel(ctx, channel.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Platform != "telegram" {
		t.Errorf("platform = %q, want %q", got.Platform, "telegram")
	}

	channels, err := repo.ListChannels(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("count = %d, want 1", len(channels))
	}

	channel.Status = "disabled"
	if err := repo.UpdateChannel(ctx, channel); err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := repo.DeleteChannel(ctx, channel.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
