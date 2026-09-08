package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

type toggledPublishBus struct {
	delegate   *bus.MemoryEventBus
	publishErr error
}

func (b *toggledPublishBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if b.publishErr != nil {
		return b.publishErr
	}
	return b.delegate.Publish(ctx, subject, event)
}

func (b *toggledPublishBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return b.delegate.Subscribe(subject, handler)
}

func (b *toggledPublishBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return b.delegate.QueueSubscribe(subject, queue, handler)
}

func (b *toggledPublishBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return b.delegate.Request(ctx, subject, event, timeout)
}

func (b *toggledPublishBus) Close() {
	b.delegate.Close()
}

func (b *toggledPublishBus) IsConnected() bool {
	return b.delegate.IsConnected()
}

func TestFireTrigger_PublishFailureFailsRunAndReleasesSlot(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	delegate := bus.NewMemoryEventBus(log)
	publishErr := errors.New("publish unavailable")
	eventBus := &toggledPublishBus{delegate: delegate, publishErr: publishErr}
	service := NewService(store, eventBus, log)

	automation := &Automation{
		WorkspaceID:        "ws-1",
		Name:               "Reusable automation",
		Enabled:            true,
		MaxConcurrentRuns:  1,
		ContinuationPolicy: ContinuationPolicyReuseThread,
	}
	if err := store.CreateAutomation(ctx, automation); err != nil {
		t.Fatal(err)
	}
	trigger := &AutomationTrigger{
		AutomationID: automation.ID,
		Type:         TriggerTypeScheduled,
		Config:       json.RawMessage(`{"expression":"* * * * *"}`),
		Enabled:      true,
	}
	if err := store.CreateTrigger(ctx, trigger); err != nil {
		t.Fatal(err)
	}

	if _, err := service.FireTrigger(ctx, automation.ID, trigger.ID, trigger.Type, json.RawMessage(`{}`), "first"); err == nil {
		t.Fatal("expected publish failure")
	} else if !strings.Contains(err.Error(), publishErr.Error()) {
		t.Fatalf("publish error = %q, want %q", err, publishErr)
	}

	runs, err := store.ListRuns(ctx, automation.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs after failed publish, want 1", len(runs))
	}
	if runs[0].Status != RunStatusFailed {
		t.Fatalf("failed publish run status = %q, want failed", runs[0].Status)
	}
	if !strings.Contains(runs[0].ErrorMessage, publishErr.Error()) {
		t.Fatalf("failed publish run error = %q, want %q", runs[0].ErrorMessage, publishErr)
	}
	current, err := store.GetAutomation(ctx, automation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.LastTriggeredAt != nil {
		t.Fatalf("last_triggered_at = %v after failed publish, want nil", current.LastTriggeredAt)
	}

	eventBus.publishErr = nil
	second, err := service.FireTrigger(ctx, automation.ID, trigger.ID, trigger.Type, json.RawMessage(`{}`), "second")
	if err != nil {
		t.Fatalf("second firing: %v", err)
	}
	if second.Skipped {
		t.Fatalf("second firing was skipped: %s", second.Reason)
	}
	if second.RunID == "" {
		t.Fatal("second firing did not return a run ID")
	}
	current, err = store.GetAutomation(ctx, automation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.LastTriggeredAt == nil {
		t.Fatal("last_triggered_at is nil after successful publish")
	}

	runs, err = store.ListRuns(ctx, automation.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs after retry, want 2", len(runs))
	}
	for _, run := range runs {
		if run.ID == second.RunID && run.Status != RunStatusTriggered {
			t.Fatalf("retry run status = %q, want triggered", run.Status)
		}
	}
}
