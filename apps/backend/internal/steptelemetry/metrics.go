package steptelemetry

import (
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Metric event names live under the telemetry.metric.* namespace so a
// log-aggregation rule can match on the event name without scanning free
// text — the routing.metric.* precedent in internal/office/scheduler.
const (
	metricStepTransitionWritten = "telemetry.metric.step_transition_written"
	metricTurnStamped           = "telemetry.metric.turn_stamped"
)

// RecordLedgerRow bumps the expvar counter for trigger AND emits a
// telemetry.metric.step_transition_written log line. Called once per
// successfully written task_step_transitions row.
func RecordLedgerRow(log *logger.Logger, trigger Trigger) {
	incStepTransition(trigger)
	if log == nil {
		return
	}
	log.Info(metricStepTransitionWritten, zap.String("trigger", string(trigger)))
}

// RecordTurnStamp bumps the expvar counter keyed by whether a turn carried
// the workflow_step_id_at_start stamp, AND emits a telemetry.metric.
// turn_stamped log line. Called once per turn created.
func RecordTurnStamp(log *logger.Logger, present bool) {
	incTurnStamp(present)
	if log == nil {
		return
	}
	log.Info(metricTurnStamped, zap.Bool("present", present))
}
