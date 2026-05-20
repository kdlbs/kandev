package channels

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// RunResolver resolves the originating office run id for a task, used
// to attribute relayed-comment activity rows back to the run that
// produced them. Optional dependency — callers without a resolver
// pass nil and activity rows are logged with empty run_id.
type RunResolver interface {
	ResolveRunForTask(ctx context.Context, taskID string) string
}

// ChannelService manages channel CRUD and inbound message routing.
type ChannelService struct {
	repo     *sqlite.Repository
	logger   *logger.Logger
	activity shared.ActivityLogger
	agents   shared.AgentReader
	runs     RunResolver
	eb       bus.EventBus
}

// SetEventBus wires an event bus for publishing comment events.
// Called after construction when the event bus is available.
func (s *ChannelService) SetEventBus(eb bus.EventBus) {
	s.eb = eb
}

// SetRunResolver wires the run resolver for attributing relayed-
// comment activity to the originating office run. Optional — leaving
// it nil keeps activity rows with empty run_id.
func (s *ChannelService) SetRunResolver(runs RunResolver) {
	s.runs = runs
}

// NewChannelService creates a new ChannelService.
func NewChannelService(
	repo *sqlite.Repository,
	log *logger.Logger,
	activity shared.ActivityLogger,
	agents shared.AgentReader,
) *ChannelService {
	return &ChannelService{
		repo:     repo,
		logger:   log.WithFields(zap.String("component", "channels-service")),
		activity: activity,
		agents:   agents,
	}
}

// SetupChannel creates a channel and its associated long-lived channel task.
func (s *ChannelService) SetupChannel(ctx context.Context, channel *models.Channel) error {
	if channel.AgentProfileID == "" {
		return fmt.Errorf("agent_profile_id is required")
	}
	if channel.Platform == "" {
		return fmt.Errorf("platform is required")
	}
	if channel.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	agent, err := s.agents.GetAgentInstance(ctx, channel.AgentProfileID)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	if channel.Status == "" {
		channel.Status = "active"
	}
	if channel.WebhookSecret == "" {
		channel.WebhookSecret = generateWebhookSecret()
	}

	// Create the channel row first (without task_id).
	if err := s.repo.CreateChannel(ctx, channel); err != nil {
		return fmt.Errorf("create channel: %w", err)
	}

	// Create a long-lived channel task.
	title := fmt.Sprintf("[%s] Channel - %s", channel.Platform, agent.Name)
	taskID, err := s.repo.CreateChannelTask(ctx, channel.WorkspaceID, title, channel.AgentProfileID)
	if err != nil {
		// Clean up the channel row on task creation failure.
		_ = s.repo.DeleteChannel(ctx, channel.ID)
		return fmt.Errorf("create channel task: %w", err)
	}

	// ADR 0005 Wave F — set the channel agent as the task's runner via
	// workflow_step_participants. Channel tasks have no workflow_step_id;
	// the participant row is keyed at (step_id="", task_id) and the
	// runner projection still resolves it.
	if err := s.repo.UpdateTaskAssignee(ctx, taskID, channel.AgentProfileID); err != nil {
		_ = s.repo.DeleteChannel(ctx, channel.ID)
		return fmt.Errorf("set channel task assignee: %w", err)
	}

	// Link the task back to the channel.
	channel.TaskID = taskID
	if err := s.repo.UpdateChannel(ctx, channel); err != nil {
		return fmt.Errorf("link task to channel: %w", err)
	}

	s.logger.Info("channel setup complete",
		zap.String("channel_id", channel.ID),
		zap.String("platform", string(channel.Platform)),
		zap.String("task_id", taskID))

	s.activity.LogActivity(ctx, channel.WorkspaceID, "user", "",
		"channel_created", "channel", channel.ID,
		fmt.Sprintf("platform=%s agent=%s", channel.Platform, agent.Name))

	return nil
}

func generateWebhookSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// GetChannelByID returns a channel by ID.
func (s *ChannelService) GetChannelByID(ctx context.Context, id string) (*models.Channel, error) {
	return s.repo.GetChannel(ctx, id)
}

// ListChannelsByAgent returns all channels for an agent instance.
func (s *ChannelService) ListChannelsByAgent(ctx context.Context, agentInstanceID string) ([]*models.Channel, error) {
	return s.repo.ListChannelsByAgent(ctx, agentInstanceID)
}

// UpdateChannelStatus updates a channel's status.
func (s *ChannelService) UpdateChannelStatus(ctx context.Context, id, status string) error {
	channel, err := s.repo.GetChannel(ctx, id)
	if err != nil {
		return err
	}
	channel.Status = models.ChannelStatus(status)
	return s.repo.UpdateChannel(ctx, channel)
}

// DeleteChannel deletes a channel by ID.
func (s *ChannelService) DeleteChannel(ctx context.Context, id string) error {
	return s.repo.DeleteChannel(ctx, id)
}

// HandleChannelInbound processes an inbound message on a channel,
// creating a comment on the channel task.
func (s *ChannelService) HandleChannelInbound(
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
		Source:         string(channel.Platform),
		ReplyChannelID: channel.ID,
	}
	return s.CreateComment(ctx, comment)
}
