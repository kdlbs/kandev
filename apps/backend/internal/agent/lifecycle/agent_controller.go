// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// AgentController handles agent control operations including starting,
// prompting, and ACP session initialization.
type AgentController struct {
	registry        *registry.Registry
	profileResolver ProfileResolver
	commandBuilder  *CommandBuilder
	sessionManager  *SessionManager
	streamManager   *StreamManager
	eventPublisher  *EventPublisher
	logger          *logger.Logger
}

// NewAgentController creates a new AgentController with the given dependencies.
func NewAgentController(
	registry *registry.Registry,
	profileResolver ProfileResolver,
	commandBuilder *CommandBuilder,
	sessionManager *SessionManager,
	streamManager *StreamManager,
	eventPublisher *EventPublisher,
	log *logger.Logger,
) *AgentController {
	return &AgentController{
		registry:        registry,
		profileResolver: profileResolver,
		commandBuilder:  commandBuilder,
		sessionManager:  sessionManager,
		streamManager:   streamManager,
		eventPublisher:  eventPublisher,
		logger:          log.WithFields(zap.String("component", "agent-controller")),
	}
}

// StartAgentProcess configures and starts the agent subprocess for an instance.
// This waits for agentctl to be ready, configures the command, starts the agent,
// and initializes the ACP session.
func (ac *AgentController) StartAgentProcess(ctx context.Context, instance *AgentInstance) error {
	if instance.agentctl == nil {
		return fmt.Errorf("instance %q has no agentctl client", instance.ID)
	}

	if instance.AgentCommand == "" {
		return fmt.Errorf("instance %q has no agent command configured", instance.ID)
	}

	// Wait for agentctl to be ready
	if err := instance.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		return fmt.Errorf("agentctl not ready: %w", err)
	}

	// Get task description from metadata
	taskDescription := ""
	if instance.Metadata != nil {
		if desc, ok := instance.Metadata["task_description"].(string); ok {
			taskDescription = desc
		}
	}

	// Build environment for the agent process
	env := map[string]string{}
	if taskDescription != "" {
		env["TASK_DESCRIPTION"] = taskDescription
	}

	// Configure the agent command
	if err := instance.agentctl.ConfigureAgent(ctx, instance.AgentCommand, env); err != nil {
		return fmt.Errorf("failed to configure agent: %w", err)
	}

	// Start the agent process
	if err := instance.agentctl.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	ac.logger.Info("agent process started",
		zap.String("instance_id", instance.ID),
		zap.String("task_id", instance.TaskID),
		zap.String("command", instance.AgentCommand))

	// Give the agent process a moment to initialize
	time.Sleep(500 * time.Millisecond)

	// Get agent config for ACP session initialization
	agentConfig, err := ac.getAgentConfigForInstance(instance)
	if err != nil {
		return fmt.Errorf("failed to get agent config: %w", err)
	}

	// Initialize ACP session
	if err := ac.initializeACPSession(ctx, instance, agentConfig, taskDescription); err != nil {
		return fmt.Errorf("failed to initialize ACP: %w", err)
	}

	return nil
}

// getAgentConfigForInstance retrieves the agent configuration for an instance.
func (ac *AgentController) getAgentConfigForInstance(instance *AgentInstance) (*registry.AgentTypeConfig, error) {
	if instance.AgentProfileID == "" {
		// Return default config if no profile
		return ac.registry.GetAgentType("augment-agent")
	}

	if ac.profileResolver == nil {
		return ac.registry.GetAgentType("augment-agent")
	}

	profileInfo, err := ac.profileResolver.ResolveProfile(context.Background(), instance.AgentProfileID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve profile: %w", err)
	}

	// Map agent name to registry ID (e.g., "auggie" -> "auggie-agent")
	agentTypeName := profileInfo.AgentName + "-agent"
	agentConfig, err := ac.registry.GetAgentType(agentTypeName)
	if err != nil {
		return nil, fmt.Errorf("agent type not found: %s", agentTypeName)
	}

	return agentConfig, nil
}

// initializeACPSession sends the ACP initialization messages using the SessionManager.
// The agentConfig is used for configuration-driven session resumption behavior.
func (ac *AgentController) initializeACPSession(ctx context.Context, instance *AgentInstance, agentConfig *registry.AgentTypeConfig, taskDescription string) error {
	ac.logger.Info("initializing ACP session",
		zap.String("instance_id", instance.ID),
		zap.String("agentctl_url", instance.agentctl.BaseURL()),
		zap.String("agent_type", agentConfig.ID))

	// Use SessionManager for configuration-driven session initialization
	result, err := ac.sessionManager.InitializeSession(
		ctx,
		instance.agentctl,
		agentConfig,
		instance.ACPSessionID, // Existing session ID to resume (if any)
		instance.WorkspacePath,
	)
	if err != nil {
		ac.logger.Error("session initialization failed",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		return err
	}

	ac.logger.Info("ACP session initialized",
		zap.String("instance_id", instance.ID),
		zap.String("agent_name", result.AgentName),
		zap.String("agent_version", result.AgentVersion),
		zap.String("session_id", result.SessionID))

	instance.ACPSessionID = result.SessionID
	ac.eventPublisher.PublishACPSessionCreated(instance, result.SessionID)

	// Set up WebSocket streams using StreamManager
	// Use a ready channel to signal when the updates stream is connected
	updatesReady := make(chan struct{})
	ac.streamManager.ConnectAll(instance, updatesReady)

	// Wait for the updates stream to connect before sending prompt
	select {
	case <-updatesReady:
		ac.logger.Debug("updates stream ready")
	case <-time.After(5 * time.Second):
		ac.logger.Warn("timeout waiting for updates stream to connect, proceeding anyway")
	}

	// Send the task prompt if provided
	if taskDescription != "" {
		ac.logger.Info("sending ACP prompt",
			zap.String("instance_id", instance.ID),
			zap.String("session_id", instance.ACPSessionID),
			zap.String("task_description", taskDescription))

		// Clear buffers before starting prompt
		instance.messageMu.Lock()
		instance.messageBuffer.Reset()
		instance.reasoningBuffer.Reset()
		instance.summaryBuffer.Reset()
		instance.messageMu.Unlock()

		// Prompt is SYNCHRONOUS - it blocks until the agent completes the task
		// Use a long timeout context for this
		promptCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		resp, err := instance.agentctl.Prompt(promptCtx, taskDescription)
		if err != nil {
			ac.logger.Error("ACP prompt failed",
				zap.String("instance_id", instance.ID),
				zap.Error(err))
			return fmt.Errorf("prompt failed: %w", err)
		}

		// Extract accumulated content from buffers
		instance.messageMu.Lock()
		agentMessage := instance.messageBuffer.String()
		instance.messageBuffer.Reset()
		instance.reasoningBuffer.Reset()
		instance.summaryBuffer.Reset()
		instance.messageMu.Unlock()

		stopReason := ""
		if resp != nil {
			stopReason = string(resp.StopReason)
		}
		ac.logger.Info("ACP prompt completed",
			zap.String("instance_id", instance.ID),
			zap.String("stop_reason", stopReason))

		// Publish prompt_complete event with the agent's response
		ac.eventPublisher.PublishPromptComplete(instance, agentMessage, "", "")

		// Mark agent as READY for follow-up prompts
		instance.Status = v1.AgentStatusReady
		ac.logger.Info("agent ready for follow-up prompts",
			zap.String("instance_id", instance.ID))
	} else {
		ac.logger.Warn("no task description provided, marking as ready",
			zap.String("instance_id", instance.ID))
		instance.Status = v1.AgentStatusReady
	}

	return nil
}

// PromptAgent sends a follow-up prompt to a running agent.
func (ac *AgentController) PromptAgent(ctx context.Context, instance *AgentInstance, prompt string) (*PromptResult, error) {
	if instance.agentctl == nil {
		return nil, fmt.Errorf("instance %q has no agentctl client", instance.ID)
	}

	// Accept both RUNNING (initial) and READY (after first prompt) states
	if instance.Status != v1.AgentStatusRunning && instance.Status != v1.AgentStatusReady {
		return nil, fmt.Errorf("instance %q is not ready for prompts (status: %s)", instance.ID, instance.Status)
	}

	// Set status to RUNNING while processing
	instance.Status = v1.AgentStatusRunning

	// Clear buffers before starting new prompt
	instance.messageMu.Lock()
	instance.messageBuffer.Reset()
	instance.reasoningBuffer.Reset()
	instance.summaryBuffer.Reset()
	instance.messageMu.Unlock()

	ac.logger.Info("sending prompt to agent",
		zap.String("instance_id", instance.ID),
		zap.Int("prompt_length", len(prompt)))

	// Prompt is synchronous - blocks until agent completes
	resp, err := instance.agentctl.Prompt(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Extract accumulated content from buffers
	instance.messageMu.Lock()
	agentMessage := instance.messageBuffer.String()
	instance.messageBuffer.Reset()
	instance.reasoningBuffer.Reset()
	instance.summaryBuffer.Reset()
	instance.messageMu.Unlock()

	result := &PromptResult{
		StopReason:   string(resp.StopReason),
		AgentMessage: agentMessage,
	}

	// Publish prompt_complete event with the agent's response
	ac.eventPublisher.PublishPromptComplete(instance, agentMessage, "", "")

	// Prompt completed - mark as READY for next prompt
	instance.Status = v1.AgentStatusReady
	ac.logger.Info("prompt completed, agent ready for follow-up",
		zap.String("instance_id", instance.ID),
		zap.String("stop_reason", result.StopReason))

	return result, nil
}

