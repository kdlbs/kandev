package stepevents

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/workflow/models"
)

type recordingBus struct {
	published []*bus.Event
	subjects  []string
	err       error
}

func (b *recordingBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.subjects = append(b.subjects, subject)
	b.published = append(b.published, event)
	return b.err
}

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	return logger.Default()
}

// fullStep returns a step whose every field is set to a distinctive non-zero
// value, so a payload that drops one is visible.
func fullStep() *models.WorkflowStep {
	created := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 8, 2, 11, 0, 0, 0, time.UTC)
	return &models.WorkflowStep{
		ID:                    "step-1",
		WorkflowID:            "workflow-1",
		Name:                  "Review",
		Position:              3,
		Color:                 "#ff00ff",
		Prompt:                "review the diff",
		Events:                models.StepEvents{OnEnter: []models.OnEnterAction{{Type: models.OnEnterAutoStartAgent}}},
		AllowManualMove:       true,
		IsStartStep:           true,
		ShowInCommandPanel:    true,
		AutoArchiveAfterHours: 24,
		AgentProfileID:        "agent-1",
		WIPLimit:              5,
		PullFromStepID:        "step-0",
		StageType:             models.StageTypeReview,

		AutoAdvanceRequiresSignal:  true,
		CancelTriggersTurnComplete: true,
		CreatedAt:                  created,
		UpdatedAt:                  updated,
	}
}

func publishOne(t *testing.T, source string, step *models.WorkflowStep) (*recordingBus, map[string]interface{}) {
	t.Helper()
	eventBus := &recordingBus{}
	NewPublisher(eventBus, source, testLogger(t)).Publish(context.Background(), events.WorkflowStepUpdated, step)
	if len(eventBus.published) != 1 {
		t.Fatalf("published %d events, want 1", len(eventBus.published))
	}
	data, ok := eventBus.published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map", eventBus.published[0].Data)
	}
	stepPayload, ok := data["step"].(map[string]interface{})
	if !ok {
		t.Fatalf("step payload type = %T, want map", data["step"])
	}
	return eventBus, stepPayload
}

func TestPublishCarriesEveryStepField(t *testing.T) {
	step := fullStep()
	_, got := publishOne(t, "test-source", step)

	want := map[string]interface{}{
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
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload mismatch:\n got = %#v\nwant = %#v", got, want)
	}
}

// TestPayloadCoversEveryModelField fails when a field is added to
// models.WorkflowStep without being added to the event payload, which would
// otherwise silently ship an incomplete step to every subscriber.
func TestPayloadCoversEveryModelField(t *testing.T) {
	got := payload(fullStep())
	stepType := reflect.TypeOf(models.WorkflowStep{})

	exported := 0
	for i := range stepType.NumField() {
		field := stepType.Field(i)
		// Only exported fields reach the wire, so unexported ones (a cached
		// value, a mutex) must not count toward the expected key total.
		if !field.IsExported() {
			continue
		}
		exported++
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			t.Fatalf("field %s has no usable json tag", field.Name)
		}
		if _, ok := got[key]; !ok {
			t.Errorf("payload is missing %q (models.WorkflowStep.%s)", key, field.Name)
		}
	}
	if len(got) != exported {
		t.Errorf("payload has %d keys, model has %d exported fields — payload carries a key with no model field",
			len(got), exported)
	}
}

func TestPublishUsesCallerSource(t *testing.T) {
	for _, source := range []string{"workflow-handlers", "mcp-handlers"} {
		eventBus, _ := publishOne(t, source, fullStep())
		if eventBus.published[0].Source != source {
			t.Errorf("source = %q, want %q", eventBus.published[0].Source, source)
		}
		if eventBus.subjects[0] != events.WorkflowStepUpdated {
			t.Errorf("subject = %q, want %q", eventBus.subjects[0], events.WorkflowStepUpdated)
		}
	}
}

func TestPublishNoOps(t *testing.T) {
	ctx := context.Background()

	t.Run("nil publisher", func(t *testing.T) {
		var p *Publisher
		p.Publish(ctx, events.WorkflowStepCreated, fullStep())
		p.PublishAll(ctx, events.WorkflowStepUpdated, []*models.WorkflowStep{fullStep()})
	})

	t.Run("nil bus", func(t *testing.T) {
		NewPublisher(nil, "test-source", testLogger(t)).Publish(ctx, events.WorkflowStepCreated, fullStep())
	})

	t.Run("nil step", func(t *testing.T) {
		eventBus := &recordingBus{}
		NewPublisher(eventBus, "test-source", testLogger(t)).Publish(ctx, events.WorkflowStepCreated, nil)
		if len(eventBus.published) != 0 {
			t.Fatalf("published %d events for a nil step, want 0", len(eventBus.published))
		}
	})
}

func TestPublishAllEmitsOnePerStep(t *testing.T) {
	eventBus := &recordingBus{}
	steps := []*models.WorkflowStep{
		{ID: "step-a", WorkflowID: "workflow-1"},
		nil,
		{ID: "step-b", WorkflowID: "workflow-1"},
	}
	NewPublisher(eventBus, "test-source", testLogger(t)).
		PublishAll(context.Background(), events.WorkflowStepUpdated, steps)

	if len(eventBus.published) != 2 {
		t.Fatalf("published %d events, want 2 (nil entries skipped)", len(eventBus.published))
	}
}

func TestPublishSurvivesBusError(t *testing.T) {
	eventBus := &recordingBus{err: errors.New("bus down")}
	NewPublisher(eventBus, "test-source", testLogger(t)).
		Publish(context.Background(), events.WorkflowStepDeleted, fullStep())

	if len(eventBus.published) != 1 {
		t.Fatalf("published %d events, want 1", len(eventBus.published))
	}
}
