package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrate/models"

	"go.uber.org/zap"
)

// SetupChannel creates a channel and its associated long-lived channel task.
func (s *Service) SetupChannel(ctx context.Context, channel *models.Channel) error {
	if channel.AgentInstanceID == "" {
		return fmt.Errorf("agent_instance_id is required")
	}
	if channel.Platform == "" {
		return fmt.Errorf("platform is required")
	}
	if channel.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	agent, err := s.repo.GetAgentInstance(ctx, channel.AgentInstanceID)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	if channel.Status == "" {
		channel.Status = "active"
	}

	// Create the channel row first (without task_id).
	if err := s.repo.CreateChannel(ctx, channel); err != nil {
		return fmt.Errorf("create channel: %w", err)
	}

	// Create a long-lived channel task.
	title := fmt.Sprintf("[%s] Channel - %s", channel.Platform, agent.Name)
	taskID, err := s.createChannelTask(ctx, channel.WorkspaceID, title)
	if err != nil {
		// Clean up the channel row on task creation failure.
		_ = s.repo.DeleteChannel(ctx, channel.ID)
		return fmt.Errorf("create channel task: %w", err)
	}

	// Link the task back to the channel.
	channel.TaskID = taskID
	if err := s.repo.UpdateChannel(ctx, channel); err != nil {
		return fmt.Errorf("link task to channel: %w", err)
	}

	s.logger.Info("channel setup complete",
		zap.String("channel_id", channel.ID),
		zap.String("platform", channel.Platform),
		zap.String("task_id", taskID))

	s.LogActivity(ctx, channel.WorkspaceID, "user", "",
		"channel_created", "channel", channel.ID,
		fmt.Sprintf("platform=%s agent=%s", channel.Platform, agent.Name))

	return nil
}

// createChannelTask creates a task for the channel.
// Returns the created task ID.
func (s *Service) createChannelTask(ctx context.Context, workspaceID, title string) (string, error) {
	taskID, err := s.repo.CreateChannelTask(ctx, workspaceID, title)
	if err != nil {
		return "", err
	}
	return taskID, nil
}

// GetChannelByID returns a channel by ID.
func (s *Service) GetChannelByID(ctx context.Context, id string) (*models.Channel, error) {
	return s.repo.GetChannel(ctx, id)
}

// ListChannelsByAgent returns all channels for an agent instance.
func (s *Service) ListChannelsByAgent(ctx context.Context, agentInstanceID string) ([]*models.Channel, error) {
	return s.repo.ListChannelsByAgent(ctx, agentInstanceID)
}

// UpdateChannelStatus updates a channel's status.
func (s *Service) UpdateChannelStatus(ctx context.Context, id, status string) error {
	channel, err := s.repo.GetChannel(ctx, id)
	if err != nil {
		return err
	}
	channel.Status = status
	return s.repo.UpdateChannel(ctx, channel)
}

// DeleteChannel deletes a channel by ID.
func (s *Service) DeleteChannel(ctx context.Context, id string) error {
	return s.repo.DeleteChannel(ctx, id)
}

// HandleChannelInbound processes an inbound message on a channel,
// creating a comment on the channel task.
func (s *Service) HandleChannelInbound(
	ctx context.Context, channelID, authorName, body string,
) error {
	channel, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return fmt.Errorf("channel not found: %w", err)
	}
	if channel.TaskID == "" {
		return fmt.Errorf("channel has no linked task")
	}

	comment := &models.TaskComment{
		TaskID:         channel.TaskID,
		AuthorType:     "user",
		AuthorID:       authorName,
		Body:           body,
		Source:         channel.Platform,
		ReplyChannelID: channel.ID,
	}
	return s.CreateComment(ctx, comment)
}
