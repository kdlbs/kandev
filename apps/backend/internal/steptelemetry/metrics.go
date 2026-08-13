package steptelemetry

import (
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Metric event names live under the telemetry.metric.* namespace so a
// log-aggregation rule can match on the event name without scanning free
// text — the routing.metric.* precedent in internal/office/scheduler.
const (
	metricStepTransitionInserted = "telemetry.metric.step_transition_inserted"
	metricTurnStamped            = "telemetry.metric.turn_stamped"
)

// RecordLedgerRow bumps the expvar counter for trigger AND emits a
// telemetry.metric.step_transition_inserted log line. Called once the INSERT
// succeeds, before the owning transaction commits — see the call site's
// comment for why this is a liveness signal, not a commit-confirmed count.
func RecordLedgerRow(log *logger.Logger, trigger Trigger) {
	incStepTransition(trigger)
	if log == nil {
		return
	}
	log.Info(metricStepTransitionInserted, zap.String("trigger", string(trigger)))
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
