// Package stepevents publishes workflow-step lifecycle events onto the shared
// event bus.
//
// Two surfaces mutate workflow steps — the REST/WS handlers in
// internal/workflow/handlers and the MCP handlers in internal/mcp/handlers —
// and their subscribers (the WS gateway fan-out in internal/gateway/websocket
// and the orchestrator queue reconciler) must observe the same payload
// regardless of which surface performed the mutation. Owning the payload here
// keeps the two from drifting; each surface supplies only its own source label.
package stepevents

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/workflow/models"
)

// Bus is the publish-only slice of bus.EventBus this package needs. Both
// bus.EventBus and the narrower EventBus declared by the MCP handlers satisfy
// it, so either surface can hand over whatever it was wired with.
type Bus interface {
	Publish(ctx context.Context, subject string, event *bus.Event) error
}

// Publisher emits workflow-step events tagged with a caller-supplied source.
type Publisher struct {
	bus    Bus
	source string
	logger *logger.Logger
}

// NewPublisher returns a Publisher that stamps every event with source, which
// is the publishing surface's provenance label (for example "mcp-handlers").
func NewPublisher(eventBus Bus, source string, log *logger.Logger) *Publisher {
	return &Publisher{bus: eventBus, source: source, logger: log}
}

// Publish emits eventType carrying step. A nil publisher, a publisher wired
// without an event bus, and a nil step are all no-ops.
func (p *Publisher) Publish(ctx context.Context, eventType string, step *models.WorkflowStep) {
	if p == nil || p.bus == nil || step == nil {
		return
	}
	data := map[string]interface{}{"step": payload(step)}
	if err := p.bus.Publish(ctx, eventType, bus.NewEvent(eventType, p.source, data)); err != nil && p.logger != nil {
		p.logger.Error("failed to publish workflow step event",
			zap.String("event_type", eventType),
			zap.String("step_id", step.ID),
			zap.Error(err))
	}
}

// PublishAll emits eventType once per step, skipping nil entries.
func (p *Publisher) PublishAll(ctx context.Context, eventType string, steps []*models.WorkflowStep) {
	for _, step := range steps {
		p.Publish(ctx, eventType, step)
	}
}

// payload builds the wire representation of a step carried by every
// workflow_step.* event. Adding a field here reaches both surfaces at once.
func payload(step *models.WorkflowStep) map[string]interface{} {
	return map[string]interface{}{
		"id":                            step.ID,
		"workflow_id":                   step.WorkflowID,
		"name":                          step.Name,
		"position":                      step.Position,
		"color":                         step.Color,
		"prompt":                        step.Prompt,
		"events":                        step.Events,
		"show_in_command_panel":         step.ShowInCommandPanel,
		"allow_manual_move":             step.AllowManualMove,
		"is_start_step":                 step.IsStartStep,
		"auto_archive_after_hours":      step.AutoArchiveAfterHours,
		"wip_limit":                     step.WIPLimit,
		"pull_from_step_id":             step.PullFromStepID,
		"agent_profile_id":              step.AgentProfileID,
		"stage_type":                    string(step.StageType),
		"auto_advance_requires_signal":  step.AutoAdvanceRequiresSignal,
		"cancel_triggers_turn_complete": step.CancelTriggersTurnComplete,
		"created_at":                    step.CreatedAt,
		"updated_at":                    step.UpdatedAt,
	}
}
