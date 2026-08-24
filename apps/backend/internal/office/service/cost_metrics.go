package service

import (
	"expvar"
	"strings"

	"go.uber.org/zap"
)

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler (dev mode). Mirrors internal/office/scheduler/metrics_vars.go's
// shape: counters only, "k1=v1;k2=v2" label keys so a future Prometheus
// translation layer can split without re-shaping storage.
//
// This pair exists because handlePromptUsage previously had two silent
// early returns (decode failure, task lookup failure) and a silent insert
// failure — "if this writer silently stops, nothing today would notice"
// (docs/specs/office/requirements/costs.md).
var (
	costEventsWrittenTotal = expvar.NewMap("cost_events_written_total")
	costEventsDroppedTotal = expvar.NewMap("cost_events_dropped_total")
)

// Reasons a prompt-usage event never became a stored cost_events row.
const (
	costDropReasonDecodeError     = "decode_error"
	costDropReasonMissingIDs      = "missing_ids"
	costDropReasonTaskFieldsError = "task_fields_error"
	costDropReasonInsertError     = "insert_error"
	costDropReasonDuplicate       = "duplicate"
)

const (
	metricCostEventWritten = "cost.metric.event_written"
	metricCostEventDropped = "cost.metric.event_dropped"
)

func costMetricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// recordCostEventWritten emits a structured log AND bumps the expvar
// counter under cost_events_written_total when handlePromptUsage
// successfully inserts a row.
func (s *Service) recordCostEventWritten(source, provider string) {
	costEventsWrittenTotal.Add(costMetricLabel("source", source, "provider", provider), 1)
	if s.logger == nil {
		return
	}
	s.logger.Debug(metricCostEventWritten,
		zap.String("cost_source", source), zap.String("provider", provider))
}

// recordCostEventDropped emits a structured log AND bumps the expvar
// counter under cost_events_dropped_total whenever a prompt-usage event
// does not result in a stored row.
func (s *Service) recordCostEventDropped(reason, taskID string) {
	costEventsDroppedTotal.Add(costMetricLabel("reason", reason), 1)
	if s.logger == nil {
		return
	}
	s.logger.Warn(metricCostEventDropped,
		zap.String("reason", reason), zap.String("task_id", taskID))
}
