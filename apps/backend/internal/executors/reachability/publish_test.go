package reachability

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func testEventBusLogger() *bus.MemoryEventBus {
	return bus.NewMemoryEventBus(logger.Default())
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.3
//
// PublishChanged must publish on events.ExecutorReachabilityChanged with a
// payload carrying the record plus the derived probing fields a client needs
// (probing_enabled/probe_interval_seconds) without a second fetch.
func TestPublisherPublishChanged_PublishesRecordDTO(t *testing.T) {
	eventBus := testEventBusLogger()
	var captured *bus.Event
	_, err := eventBus.Subscribe(events.ExecutorReachabilityChanged, func(_ context.Context, ev *bus.Event) error {
		captured = ev
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	pub := NewPublisher(eventBus)
	checkedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	record := &models.ExecutorReachability{
		ExecutorID: "exec-1",
		State:      models.ExecutorReachabilityStateUnreachable,
		Reason:     models.ExecutorReachabilityReasonNetwork,
		Message:    "connection refused",
		Host:       "10.0.0.1",
		CheckedAt:  &checkedAt,
	}
	pub.PublishChanged(context.Background(), record, 60)

	if captured == nil {
		t.Fatal("no event was published")
	}
	dto, ok := captured.Data.(RecordDTO)
	if !ok {
		t.Fatalf("event data type = %T, want RecordDTO", captured.Data)
	}
	if dto.ExecutorID != "exec-1" || dto.State != "unreachable" || dto.Reason != "network" {
		t.Fatalf("dto = %+v, want executor-1/unreachable/network", dto)
	}
	if !dto.ProbingEnabled || dto.ProbeIntervalSeconds != 60 {
		t.Fatalf("dto probing fields = %+v, want enabled/60", dto)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.14
//
// probing_enabled must be false exactly when the effective interval is 0 —
// the client's only way to distinguish "not probed yet" from "probing off".
func TestPublisherPublishChanged_ProbingDisabledWhenIntervalIsZero(t *testing.T) {
	eventBus := testEventBusLogger()
	var captured *bus.Event
	_, _ = eventBus.Subscribe(events.ExecutorReachabilityChanged, func(_ context.Context, ev *bus.Event) error {
		captured = ev
		return nil
	})

	pub := NewPublisher(eventBus)
	pub.PublishChanged(context.Background(), &models.ExecutorReachability{ExecutorID: "exec-1"}, 0)

	dto := captured.Data.(RecordDTO)
	if dto.ProbingEnabled {
		t.Fatalf("ProbingEnabled = true, want false when the effective interval is 0")
	}
}

// A nil publisher, nil record, or nil bus must never panic — PublishChanged
// is called from probe/save-reset paths that don't want a defensive nil
// check at every call site.
func TestPublisherPublishChanged_NilSafe(t *testing.T) {
	var nilPub *Publisher
	nilPub.PublishChanged(context.Background(), &models.ExecutorReachability{ExecutorID: "exec-1"}, 60)

	pub := NewPublisher(testEventBusLogger())
	pub.PublishChanged(context.Background(), nil, 60)

	pub2 := NewPublisher(nil)
	pub2.PublishChanged(context.Background(), &models.ExecutorReachability{ExecutorID: "exec-1"}, 60)
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.14, AC-EXECUTORS-SSH-REACHABILITY-002.5
//
// BuildRecordDTO is the single place the HTTP routes and the event publisher
// share for projecting a stored (or absent) record into the wire shape —
// including the "no record yet" unknown placeholder with every timestamp
// null, never omitted.
func TestBuildRecordDTO_SynthesizesUnknownShapeWhenRecordIsNil(t *testing.T) {
	dto := BuildRecordDTO("exec-1", nil, 60, true)
	if dto.ExecutorID != "exec-1" || dto.State != string(models.ExecutorReachabilityStateUnknown) || dto.Reason != "" {
		t.Fatalf("dto = %+v, want a synthesized unknown/empty-reason shape", dto)
	}
	if dto.CheckedAt != nil || dto.LastSuccessAt != nil || dto.UpdatedAt != nil {
		t.Fatalf("dto = %+v, want every timestamp nil", dto)
	}
	if dto.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures = %d, want 0", dto.ConsecutiveFailures)
	}
}

func TestBuildRecordDTO_ProjectsStoredRecord(t *testing.T) {
	checkedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	record := &models.ExecutorReachability{
		ExecutorID:          "exec-1",
		State:               models.ExecutorReachabilityStateReachable,
		ConsecutiveFailures: 0,
		Host:                "10.0.0.1",
		CheckedAt:           &checkedAt,
		LastSuccessAt:       &checkedAt,
		UpdatedAt:           checkedAt,
	}
	dto := BuildRecordDTO("exec-1", record, 60, true)
	if dto.State != "reachable" || dto.Host != "10.0.0.1" {
		t.Fatalf("dto = %+v, want the stored record projected", dto)
	}
	if dto.CheckedAt == nil || !dto.CheckedAt.Equal(checkedAt) {
		t.Fatalf("CheckedAt = %v, want %v", dto.CheckedAt, checkedAt)
	}
	if !dto.Persisted {
		t.Fatalf("Persisted = false, want true")
	}
}
