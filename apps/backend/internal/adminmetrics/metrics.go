// Package adminmetrics exposes the durable administrative-turn counters used
// by delivery recovery, completion settlement, and stale-turn controls.
package adminmetrics

import (
	"expvar"
	"strings"
)

var (
	completionIntentsPending    = expvar.NewInt("administrative_turn_completion_intents_pending")
	completionReconciledTotal   = expvar.NewMap("administrative_turn_completion_reconciled_total")
	messageDeliveryOutcomes     = expvar.NewMap("administrative_turn_message_delivery_outcomes_total")
	messageDeliveryRetriesTotal = expvar.NewInt("administrative_turn_message_delivery_retries_total")
	messageDeliveryDuplicates   = expvar.NewInt("administrative_turn_message_delivery_duplicates_total")
	staleControlDenialsTotal    = expvar.NewMap("administrative_turn_stale_control_denials_total")
)

func RecordCompletionPending(count int) {
	if count < 0 {
		count = 0
	}
	completionIntentsPending.Set(int64(count))
}

func RecordCompletionReconciled(outcome, cause string) {
	completionReconciledTotal.Add(metricLabel("outcome", outcome, "cause", cause), 1)
}

func RecordMessageDeliveryOutcome(outcome string, count int64) {
	if count > 0 {
		messageDeliveryOutcomes.Add(metricLabel("outcome", outcome), count)
	}
}

func RecordMessageDeliveryRetry() { messageDeliveryRetriesTotal.Add(1) }

func RecordMessageDeliveryDuplicate() { messageDeliveryDuplicates.Add(1) }

func RecordStaleControlDenial(reason string) {
	staleControlDenialsTotal.Add(metricLabel("reason", reason), 1)
}

func metricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}
