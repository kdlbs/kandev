package coordinator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// DreamPrompter sends one prompt to a session and returns the agent's last
// message. Satisfied by the orchestrator through PromptDreamEpisode.
type DreamPrompter func(ctx context.Context, taskID, sessionID, prompt string) (string, error)

// PromptDreamEpisode adapts orchestrator.PromptTask to a DreamPrompter.
func PromptDreamEpisode(o interface {
	PromptTask(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool) (*orchestrator.PromptResult, error)
}) DreamPrompter {
	return func(ctx context.Context, taskID, sessionID, prompt string) (string, error) {
		res, err := o.PromptTask(ctx, taskID, sessionID, prompt, "", false, nil, false)
		if err != nil {
			return "", err
		}
		if res == nil {
			return "", errors.New("dream episode: empty prompt result")
		}
		return res.AgentMessage, nil
	}
}

// DreamEpisode creates, drives and archives the ephemeral dream episode task.
// Its binding lists exactly one tool, the turn ledger reader.
type DreamEpisode struct {
	Tasks    ConversationTaskManager
	Sessions SessionEnsurer
	Prompter DreamPrompter
}

// CreateTask creates the coordinator-origin episode task with the one-tool
// binding and returns its id.
func (e DreamEpisode) CreateTask(ctx context.Context, c *Coordinator) (string, error) {
	metadata := map[string]interface{}{
		taskmodels.MetaKeyCoordinatorID:      c.ID,
		taskmodels.MetaKeyCoordinatorPurpose: taskmodels.CoordinatorPurposeDream,
		taskmodels.MetaKeyAgentProfileID:     c.AgentProfileID,
		taskmodels.MetaKeyExecutorProfileID:  c.ExecutorProfileID,
	}
	created, err := e.Tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:           c.WorkspaceID,
		Title:                 "Coordinator dream: " + c.Name,
		IsEphemeral:           true,
		Origin:                taskmodels.TaskOriginCoordinator,
		Metadata:              metadata,
		AllowReservedMetadata: true,
	})
	if err != nil {
		return "", fmt.Errorf("create dream episode task: %w", err)
	}
	id := created.Task.ID
	if err := e.bind(ctx, c, id, metadata); err != nil {
		return id, err
	}
	return id, nil
}

func (e DreamEpisode) bind(ctx context.Context, c *Coordinator, taskID string, metadata map[string]interface{}) error {
	encoded, err := profile.MarshalCoordinatorToolPolicy(profile.CoordinatorToolPolicy{
		Version:            1,
		CoordinatorID:      c.ID,
		WorkspaceID:        c.WorkspaceID,
		ConversationTaskID: taskID,
		PolicyRevision:     c.PolicyRevision,
		ToolNames:          []string{turnsTool},
	})
	if err != nil {
		return fmt.Errorf("bind dream episode task: %w", err)
	}
	metadata[profile.CoordinatorToolPolicyMetadataKey] = encoded
	if _, err := e.Tasks.UpdateTask(ctx, taskID, &taskservice.UpdateTaskRequest{
		Metadata: metadata, AllowReservedMetadata: true,
	}); err != nil {
		return fmt.Errorf("bind dream episode task: %w", err)
	}
	return nil
}

// CreateSession ensures the episode task's session without starting an agent.
func (e DreamEpisode) CreateSession(ctx context.Context, taskID string) (string, error) {
	autoStart := false
	res, err := e.Sessions.EnsureSession(ctx, taskID, orchestrator.EnsureSessionOptions{
		AutoStart:        &autoStart,
		ActivationSource: orchestrator.LaunchActivationSourceSessionOpen,
	})
	if err != nil {
		return "", fmt.Errorf("create dream episode session: %w", err)
	}
	if res == nil || res.SessionID == "" {
		return "", errors.New("create dream episode session: no session id")
	}
	return res.SessionID, nil
}

// Archive archives the episode task, stopping its running turn; a task that
// no longer exists or is already archived counts as archived.
func (e DreamEpisode) Archive(ctx context.Context, taskID string) error {
	err := e.Tasks.ArchiveTask(ctx, taskID)
	if err == nil || errors.Is(err, taskrepo.ErrTaskNotFound) || errors.Is(err, taskservice.ErrTaskAlreadyArchived) {
		return nil
	}
	return err
}

// Prompt sends the message to the episode session and returns the agent's last
// message.
func (e DreamEpisode) Prompt(ctx context.Context, taskID, sessionID, message string) (string, error) {
	return e.Prompter(ctx, taskID, sessionID, message)
}
