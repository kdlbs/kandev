package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// Service coordinates automation operations.
type Service struct {
	store    *Store
	eventBus bus.EventBus
	logger   *logger.Logger
}

// NewService creates a new automation service.
func NewService(store *Store, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		store:    store,
		eventBus: eventBus,
		logger:   log,
	}
}

// Store returns the underlying store (for scheduler/poller access).
func (s *Service) Store() *Store {
	return s.store
}

// --- Automation CRUD ---

// CreateAutomation creates an automation with its initial triggers.
func (s *Service) CreateAutomation(ctx context.Context, req *CreateAutomationRequest) (*Automation, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if req.WorkflowID == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}
	if req.WorkflowStepID == "" {
		return nil, fmt.Errorf("workflow_step_id is required")
	}

	maxRuns := req.MaxConcurrentRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}

	a := &Automation{
		WorkspaceID:       req.WorkspaceID,
		Name:              req.Name,
		Description:       req.Description,
		WorkflowID:        req.WorkflowID,
		WorkflowStepID:    req.WorkflowStepID,
		AgentProfileID:    req.AgentProfileID,
		ExecutorProfileID: req.ExecutorProfileID,
		Prompt:            req.Prompt,
		TaskTitleTemplate: req.TaskTitleTemplate,
		Enabled:           true,
		MaxConcurrentRuns: maxRuns,
	}
	if err := s.store.CreateAutomation(ctx, a); err != nil {
		return nil, fmt.Errorf("create automation: %w", err)
	}

	// Create initial triggers.
	for _, ts := range req.Triggers {
		t := &AutomationTrigger{
			AutomationID: a.ID,
			Type:         ts.Type,
			Config:       ts.Config,
			Enabled:      ts.Enabled,
		}
		if err := s.store.CreateTrigger(ctx, t); err != nil {
			s.logger.Error("failed to create trigger during automation creation",
				zap.String("automation_id", a.ID),
				zap.String("type", string(ts.Type)),
				zap.Error(err))
		}
	}

	return s.store.GetAutomation(ctx, a.ID)
}

// GetAutomation retrieves an automation by ID.
func (s *Service) GetAutomation(ctx context.Context, id string) (*Automation, error) {
	return s.store.GetAutomation(ctx, id)
}

// ListAutomations returns all automations for a workspace.
func (s *Service) ListAutomations(ctx context.Context, workspaceID string) ([]*Automation, error) {
	return s.store.ListAutomations(ctx, workspaceID)
}

// UpdateAutomation applies partial updates.
func (s *Service) UpdateAutomation(ctx context.Context, id string, req *UpdateAutomationRequest) (*Automation, error) {
	if err := s.store.UpdateAutomation(ctx, id, req); err != nil {
		return nil, err
	}
	return s.store.GetAutomation(ctx, id)
}

// DeleteAutomation removes an automation.
func (s *Service) DeleteAutomation(ctx context.Context, id string) error {
	return s.store.DeleteAutomation(ctx, id)
}

// EnableAutomation sets enabled = true.
func (s *Service) EnableAutomation(ctx context.Context, id string) error {
	enabled := true
	return s.store.UpdateAutomation(ctx, id, &UpdateAutomationRequest{Enabled: &enabled})
}

// DisableAutomation sets enabled = false.
func (s *Service) DisableAutomation(ctx context.Context, id string) error {
	enabled := false
	return s.store.UpdateAutomation(ctx, id, &UpdateAutomationRequest{Enabled: &enabled})
}

// --- Trigger CRUD ---

// AddTrigger adds a trigger to an automation.
func (s *Service) AddTrigger(ctx context.Context, req *AddTriggerRequest) (*AutomationTrigger, error) {
	if req.AutomationID == "" {
		return nil, fmt.Errorf("automation_id is required")
	}
	t := &AutomationTrigger{
		AutomationID: req.AutomationID,
		Type:         req.Type,
		Config:       req.Config,
		Enabled:      req.Enabled,
	}
	if err := s.store.CreateTrigger(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTrigger updates a trigger.
func (s *Service) UpdateTrigger(ctx context.Context, id string, req *UpdateTriggerRequest) error {
	return s.store.UpdateTrigger(ctx, id, req)
}

// DeleteTrigger removes a trigger.
func (s *Service) DeleteTrigger(ctx context.Context, id string) error {
	return s.store.DeleteTrigger(ctx, id)
}

// --- Run queries ---

// ListRuns returns recent runs for an automation.
func (s *Service) ListRuns(ctx context.Context, automationID string, limit int) ([]*AutomationRun, error) {
	return s.store.ListRuns(ctx, automationID, limit)
}

// --- Trigger firing ---

// FireTrigger publishes an AutomationTriggered event for the given trigger.
// The orchestrator handles task creation in response.
func (s *Service) FireTrigger(ctx context.Context, automationID, triggerID string, triggerType TriggerType, triggerData json.RawMessage, dedupKey string) error {
	// Check dedup.
	if dedupKey != "" {
		exists, err := s.store.HasRunWithDedupKey(ctx, automationID, dedupKey)
		if err != nil {
			return fmt.Errorf("check dedup: %w", err)
		}
		if exists {
			s.logger.Debug("skipping duplicate trigger",
				zap.String("automation_id", automationID),
				zap.String("dedup_key", dedupKey))
			return nil
		}
	}

	evt := &AutomationTriggeredEvent{
		AutomationID: automationID,
		TriggerID:    triggerID,
		TriggerType:  triggerType,
		TriggerData:  triggerData,
		DedupKey:     dedupKey,
	}

	now := time.Now().UTC()
	if updateErr := s.store.UpdateLastTriggered(ctx, automationID, now); updateErr != nil {
		s.logger.Warn("failed to update last_triggered_at",
			zap.String("automation_id", automationID), zap.Error(updateErr))
	}
	if updateErr := s.store.UpdateTriggerEvaluatedAt(ctx, triggerID, now); updateErr != nil {
		s.logger.Warn("failed to update last_evaluated_at",
			zap.String("trigger_id", triggerID), zap.Error(updateErr))
	}

	event := bus.NewEvent(events.AutomationTriggered, "automation_service", evt)
	if err := s.eventBus.Publish(ctx, events.AutomationTriggered, event); err != nil {
		return fmt.Errorf("publish automation triggered: %w", err)
	}

	s.logger.Info("automation trigger fired",
		zap.String("automation_id", automationID),
		zap.String("trigger_id", triggerID),
		zap.String("type", string(triggerType)))
	return nil
}

// RecordRun records a trigger run outcome.
func (s *Service) RecordRun(ctx context.Context, run *AutomationRun) error {
	return s.store.CreateRun(ctx, run)
}

// GetWebhookSecret returns the webhook secret for an automation.
func (s *Service) GetWebhookSecret(ctx context.Context, id string) (string, error) {
	a, err := s.store.GetAutomation(ctx, id)
	if err != nil {
		return "", err
	}
	if a == nil {
		return "", fmt.Errorf("automation not found: %s", id)
	}
	return a.WebhookSecret, nil
}
