// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/pkg/acp/protocol"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// SimulatedAgentManagerClient simulates agent container behavior for testing.
// It publishes realistic agent events (started, ACP messages, completion) to the event bus.
type SimulatedAgentManagerClient struct {
	eventBus      bus.EventBus
	logger        *logger.Logger
	mu            sync.Mutex
	instances     map[string]*simulatedInstance
	launchDelay   time.Duration
	executionTime time.Duration
	shouldFail    bool
	failAfter     int // Fail after N successful launches
	launchCount   int32
	acpMessageFn  func(taskID, executionID string) []protocol.Message // Custom ACP messages
	stopCh        chan struct{}
}

// simulatedInstance tracks a simulated agent instance
type simulatedInstance struct {
	id             string
	taskID         string
	sessionID      string
	agentProfileID string
	status         v1.AgentStatus
	statusMu       sync.Mutex // Protects status field
	stopCh         chan struct{}
}

// NewSimulatedAgentManager creates a new simulated agent manager
func NewSimulatedAgentManager(eventBus bus.EventBus, log *logger.Logger) *SimulatedAgentManagerClient {
	return &SimulatedAgentManagerClient{
		eventBus:      eventBus,
		logger:        log,
		instances:     make(map[string]*simulatedInstance),
		launchDelay:   50 * time.Millisecond,
		executionTime: 200 * time.Millisecond,
		stopCh:        make(chan struct{}),
	}
}

// SetLaunchDelay sets the delay before agent "starts"
func (s *SimulatedAgentManagerClient) SetLaunchDelay(d time.Duration) {
	s.launchDelay = d
}

// SetExecutionTime sets how long the simulated task takes
func (s *SimulatedAgentManagerClient) SetExecutionTime(d time.Duration) {
	s.executionTime = d
}

// SetShouldFail configures whether launches should fail
func (s *SimulatedAgentManagerClient) SetShouldFail(fail bool) {
	s.shouldFail = fail
}

// SetFailAfter configures the agent to fail after N successful launches
func (s *SimulatedAgentManagerClient) SetFailAfter(n int) {
	s.failAfter = n
}

// SetACPMessageFn sets a custom function to generate ACP messages
func (s *SimulatedAgentManagerClient) SetACPMessageFn(fn func(taskID, executionID string) []protocol.Message) {
	s.acpMessageFn = fn
}

// LaunchAgent simulates launching an agent container
func (s *SimulatedAgentManagerClient) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	count := atomic.AddInt32(&s.launchCount, 1)

	if s.shouldFail {
		return nil, fmt.Errorf("simulated launch failure")
	}

	if s.failAfter > 0 && int(count) > s.failAfter {
		return nil, fmt.Errorf("simulated launch failure after %d attempts", s.failAfter)
	}

	executionID := uuid.New().String()
	containerID := "sim-container-" + executionID[:8]

	s.mu.Lock()
	instance := &simulatedInstance{
		id:             executionID,
		taskID:         req.TaskID,
		sessionID:      req.SessionID,
		agentProfileID: req.AgentProfileID,
		status:         v1.AgentStatusStarting,
		stopCh:         make(chan struct{}),
	}
	s.instances[executionID] = instance
	s.mu.Unlock()

	// Simulate agent lifecycle in background
	go s.runAgentSimulation(instance, req)

	return &executor.LaunchAgentResponse{
		AgentExecutionID: executionID,
		ContainerID:      containerID,
		Status:           v1.AgentStatusStarting,
	}, nil
}

// SetExecutionDescription updates the task description for an execution
func (s *SimulatedAgentManagerClient) SetExecutionDescription(ctx context.Context, agentExecutionID string, description string) error {
	return nil
}

// StartAgentProcess simulates starting the agent subprocess
func (s *SimulatedAgentManagerClient) StartAgentProcess(ctx context.Context, agentExecutionID string) error {
	s.logger.Info("simulated: starting agent process",
		zap.String("agent_execution_id", agentExecutionID))
	return nil
}

// runAgentSimulation simulates the agent execution lifecycle
func (s *SimulatedAgentManagerClient) runAgentSimulation(instance *simulatedInstance, req *executor.LaunchAgentRequest) {
	// Wait for launch delay
	select {
	case <-time.After(s.launchDelay):
	case <-instance.stopCh:
		return
	case <-s.stopCh:
		return
	}

	// Publish agent started event
	s.publishAgentEvent(events.AgentStarted, instance)

	// Simulate some ACP messages
	s.publishACPMessages(instance, req)

	// Wait for execution time
	select {
	case <-time.After(s.executionTime):
	case <-instance.stopCh:
		s.publishAgentEvent(events.AgentStopped, instance)
		return
	case <-s.stopCh:
		return
	}

	// Publish agent ready (finished prompt, waiting for follow-up or completion)
	instance.statusMu.Lock()
	instance.status = v1.AgentStatusReady
	instance.statusMu.Unlock()
	s.publishAgentEvent(events.AgentReady, instance)
}

// publishAgentEvent publishes an agent lifecycle event
func (s *SimulatedAgentManagerClient) publishAgentEvent(eventType string, instance *simulatedInstance) {
	instance.statusMu.Lock()
	status := instance.status
	instance.statusMu.Unlock()

	data := map[string]interface{}{
		"instance_id":      instance.id,
		"task_id":          instance.taskID,
		"agent_profile_id": instance.agentProfileID,
		"container_id":     "sim-container-" + instance.id[:8],
		"status":           string(status),
		"started_at":       time.Now(),
		"progress":         50,
	}

	event := bus.NewEvent(eventType, "simulated-agent-manager", data)
	if err := s.eventBus.Publish(context.Background(), eventType, event); err != nil {
		s.logger.Error("failed to publish agent event")
	}
}

// publishACPMessages publishes simulated ACP messages
func (s *SimulatedAgentManagerClient) publishACPMessages(instance *simulatedInstance, req *executor.LaunchAgentRequest) {
	var messages []protocol.Message

	if s.acpMessageFn != nil {
		messages = s.acpMessageFn(instance.taskID, instance.id)
	} else {
		// Default ACP messages
		messages = []protocol.Message{
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    instance.taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 25,
					"stage":    "starting",
					"message":  "Agent started processing task",
				},
			},
			{
				Type:      protocol.MessageTypeLog,
				TaskID:    instance.taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"level":   "info",
					"message": "Processing task: " + req.TaskDescription,
				},
			},
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    instance.taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 75,
					"stage":    "executing",
					"message":  "Task execution in progress",
				},
			},
		}
	}

	// Publish each message with a small delay
	for _, msg := range messages {
		sessionID := msg.SessionID
		if sessionID == "" {
			sessionID = instance.sessionID
		}
		msgData := map[string]interface{}{
			"type":       msg.Type,
			"task_id":    msg.TaskID,
			"session_id": sessionID,
			"timestamp":  msg.Timestamp,
			"data":       msg.Data,
		}

		event := bus.NewEvent(events.AgentStream, "simulated-agent", msgData)
		subject := events.BuildAgentStreamSubject(instance.taskID)
		if err := s.eventBus.Publish(context.Background(), subject, event); err != nil {
			s.logger.Error("failed to publish agent stream event")
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// StopAgent simulates stopping an agent
func (s *SimulatedAgentManagerClient) StopAgent(ctx context.Context, agentExecutionID string, force bool) error {
	s.mu.Lock()
	execution, exists := s.instances[agentExecutionID]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("agent execution %q not found", agentExecutionID)
	}

	close(execution.stopCh)
	execution.statusMu.Lock()
	execution.status = v1.AgentStatusStopped
	execution.statusMu.Unlock()

	s.publishAgentEvent(events.AgentStopped, execution)
	return nil
}

// PromptAgent sends a follow-up prompt to a running agent
// Note: attachments parameter is accepted but not used in simulation
func (s *SimulatedAgentManagerClient) PromptAgent(ctx context.Context, agentExecutionID string, prompt string, _ []v1.MessageAttachment) (*executor.PromptResult, error) {
	s.mu.Lock()
	execution, exists := s.instances[agentExecutionID]
	s.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("agent execution %q not found", agentExecutionID)
	}

	// Simulate receiving prompt and generating response
	go func() {
		time.Sleep(50 * time.Millisecond)

		msg := protocol.Message{
			Type:      protocol.MessageTypeLog,
			TaskID:    execution.taskID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"level":   "info",
				"message": "Received follow-up prompt: " + prompt,
			},
		}

		msgData := map[string]interface{}{
			"type":      msg.Type,
			"task_id":   msg.TaskID,
			"timestamp": msg.Timestamp,
			"data":      msg.Data,
		}

		event := bus.NewEvent(events.AgentStream, "simulated-agent", msgData)
		subject := events.BuildAgentStreamSubject(execution.taskID)
		if err := s.eventBus.Publish(context.Background(), subject, event); err != nil {
			s.logger.Warn("failed to publish simulated agent stream event", zap.Error(err))
		}
	}()

	return &executor.PromptResult{
		StopReason: "end_turn",
	}, nil
}

// RespondToPermissionBySessionID responds to a permission request for a session
func (s *SimulatedAgentManagerClient) RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	s.logger.Info("simulated: responding to permission",
		zap.String("session_id", sessionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))
	return nil
}

// CompleteAgent marks an agent as completed
func (s *SimulatedAgentManagerClient) CompleteAgent(executionID string) {
	s.mu.Lock()
	execution, exists := s.instances[executionID]
	s.mu.Unlock()

	if !exists {
		return
	}

	execution.status = v1.AgentStatusCompleted
	s.publishAgentEvent(events.AgentCompleted, execution)
}

// FailAgent marks an agent as failed
func (s *SimulatedAgentManagerClient) FailAgent(executionID string, reason string) {
	s.mu.Lock()
	execution, exists := s.instances[executionID]
	s.mu.Unlock()

	if !exists {
		return
	}

	execution.status = v1.AgentStatusFailed
	s.publishAgentEvent(events.AgentFailed, execution)
}

// GetLaunchCount returns the number of times LaunchAgent was called
func (s *SimulatedAgentManagerClient) GetLaunchCount() int {
	return int(atomic.LoadInt32(&s.launchCount))
}

// Close stops all simulated agents
func (s *SimulatedAgentManagerClient) Close() {
	close(s.stopCh)
}

// IsAgentRunningForSession checks if a simulated agent is running for a session
func (s *SimulatedAgentManagerClient) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, inst := range s.instances {
		if inst.sessionID == sessionID && inst.status == v1.AgentStatusRunning {
			return true
		}
	}
	return false
}

// CancelAgent cancels the current agent turn for a session
func (s *SimulatedAgentManagerClient) CancelAgent(ctx context.Context, sessionID string) error {
	s.logger.Info("simulated: cancelling agent turn",
		zap.String("session_id", sessionID))
	return nil
}

// ResolveAgentProfile resolves an agent profile ID to profile information
func (s *SimulatedAgentManagerClient) ResolveAgentProfile(ctx context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{
		ProfileID:   profileID,
		ProfileName: "Simulated Profile",
		AgentID:     "augment-agent",
		AgentName:   "Augment Agent",
		Model:       "claude-sonnet-4-20250514",
	}, nil
}

func (s *SimulatedAgentManagerClient) IsPassthroughSession(ctx context.Context, sessionID string) bool {
	return false
}
