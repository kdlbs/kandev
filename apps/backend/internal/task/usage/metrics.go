package usage

import (
	"expvar"
	"strings"

	"go.uber.org/zap"
)

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler (dev mode). Mirrors internal/office/service/cost_metrics.go's
// shape for the same reason: this ledger's writer has no other observable
// failure surface between "an event arrived on the bus" and "a row exists",
// so every terminal outcome bumps exactly one of these (AC-27).
var (
	eventsWrittenTotal = expvar.NewMap("task_usage_events_written_total")
	eventsDroppedTotal = expvar.NewMap("task_usage_events_dropped_total")
)

// Reasons a session-prompt-usage event never became a stored
// task_usage_events row (AC-27). Every terminal state-machine transition
// increments exactly one of these, via whichever of the fixed
// decode/admit/validate/ownership/insert stages the event fails first.
const (
	dropReasonDecodeError    = "decode_error"
	dropReasonUnattributable = "unattributable"
	dropReasonInvalid        = "invalid"
	dropReasonDuplicate      = "duplicate"
	dropReasonOverflow       = "overflow"
	dropReasonDrainTimeout   = "drain_timeout"
	dropReasonShutdown       = "shutdown"
	dropReasonError          = "error"
)

const (
	metricEventWritten = "task_usage.metric.event_written"
	metricEventDropped = "task_usage.metric.event_dropped"
)

func usageMetricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// recordWritten bumps task_usage_events_written_total, keyed by the row's
// resolved cost source and provider.
func (w *Writer) recordWritten(source, provider string) {
	eventsWrittenTotal.Add(usageMetricLabel("source", source, "provider", provider), 1)
	if w.log == nil {
		return
	}
	w.log.Debug(metricEventWritten, zap.String("cost_source", source), zap.String("provider", provider))
}

// recordDropped bumps task_usage_events_dropped_total, keyed by reason.
func (w *Writer) recordDropped(reason, taskID string) {
	eventsDroppedTotal.Add(usageMetricLabel("reason", reason), 1)
	if w.log == nil {
		return
	}
	w.log.Warn(metricEventDropped, zap.String("reason", reason), zap.String("task_id", taskID))
}
