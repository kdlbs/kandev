package reachability

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// RecordDTO is the wire shape every reachability HTTP route and the change
// event share, so a client's WS handler and its GET/POST response handler
// stay structurally identical. Reason is always present, even empty — the
// surface's contract is that a reachable/unknown state can carry a non-empty
// reason (a below-threshold failure), so an omitted field would be
// ambiguous with "definitely no reason".
type RecordDTO struct {
	ExecutorID           string     `json:"executor_id"`
	State                string     `json:"state"`
	Reason               string     `json:"reason"`
	Message              string     `json:"message,omitempty"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	Host                 string     `json:"host,omitempty"`
	CheckedAt            *time.Time `json:"checked_at"`
	LastSuccessAt        *time.Time `json:"last_success_at"`
	UpdatedAt            *time.Time `json:"updated_at"`
	ProbingEnabled       bool       `json:"probing_enabled"`
	ProbeIntervalSeconds int        `json:"probe_interval_seconds"`
	Persisted            bool       `json:"persisted"`
}

// BuildRecordDTO projects record into the wire shape. A nil record (an
// eligible executor never probed) synthesizes the "unknown" placeholder with
// every timestamp null, per AC-EXECUTORS-SSH-REACHABILITY-002.5 — this is
// never a 404, so every caller building a GET response for an eligible
// executor can call this unconditionally.
func BuildRecordDTO(executorID string, record *models.ExecutorReachability, effectiveIntervalSeconds int, persisted bool) RecordDTO {
	dto := RecordDTO{
		ExecutorID:           executorID,
		State:                string(models.ExecutorReachabilityStateUnknown),
		ProbingEnabled:       effectiveIntervalSeconds > 0,
		ProbeIntervalSeconds: effectiveIntervalSeconds,
		Persisted:            persisted,
	}
	if record == nil {
		return dto
	}
	dto.State = string(record.State)
	dto.Reason = string(record.Reason)
	dto.Message = record.Message
	dto.ConsecutiveFailures = record.ConsecutiveFailures
	dto.Host = record.Host
	dto.CheckedAt = record.CheckedAt
	dto.LastSuccessAt = record.LastSuccessAt
	if !record.UpdatedAt.IsZero() {
		updatedAt := record.UpdatedAt
		dto.UpdatedAt = &updatedAt
	}
	return dto
}

// Publisher publishes events.ExecutorReachabilityChanged. Constructed once
// at startup wiring and shared by the poller (probe results) and the save
// observer (configuration resets) — one propagation path for both triggers,
// per the system design.
type Publisher struct {
	eventBus bus.EventBus
	log      *logger.Logger
}

// NewPublisher builds a Publisher. A nil eventBus is accepted so a caller
// that hasn't wired the event bus yet (tests, a degraded boot path) can
// still construct one; PublishChanged becomes a no-op. An optional logger
// records publication failures with the affected executor id.
func NewPublisher(eventBus bus.EventBus, logs ...*logger.Logger) *Publisher {
	var log *logger.Logger
	if len(logs) > 0 {
		log = logs[0]
	}
	return &Publisher{eventBus: eventBus, log: log}
}

// PublishChanged publishes record's current shape on
// events.ExecutorReachabilityChanged. Safe to call on a nil *Publisher, with
// a nil record, or with a Publisher built from a nil event bus — every
// combination is a no-op rather than a panic, so callers on the probe/reset
// hot paths don't need a defensive check of their own.
func (p *Publisher) PublishChanged(ctx context.Context, record *models.ExecutorReachability, effectiveIntervalSeconds int) {
	if p == nil || p.eventBus == nil || record == nil {
		return
	}
	dto := BuildRecordDTO(record.ExecutorID, record, effectiveIntervalSeconds, true)
	event := bus.NewEvent(events.ExecutorReachabilityChanged, "executor-reachability", dto)
	if err := p.eventBus.Publish(ctx, events.ExecutorReachabilityChanged, event); err != nil {
		publishFailedTotal.Add(1)
		if p.log != nil {
			p.log.Warn("executor ssh reachability: publish failed",
				zap.String("executor_id", record.ExecutorID), zap.Error(err))
		}
	}
}
