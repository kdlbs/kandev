package journal

import (
	"errors"
	"expvar"
	"strings"
)

var (
	deliverySubmissionsTotal          = expvar.NewMap("agent_delivery_submissions_total")
	deliveryDuplicateSubmissionsTotal = expvar.NewMap("agent_delivery_duplicate_submissions_total")
	deliveryReplayedEventsTotal       = expvar.NewInt("agent_delivery_replayed_events_total")
	deliverySequenceErrorsTotal       = expvar.NewMap("agent_delivery_sequence_errors_total")
	deliveryUncertainSubmissionsTotal = expvar.NewMap("agent_delivery_uncertain_submissions_total")
	deliveryJournalErrorsTotal        = expvar.NewMap("agent_delivery_journal_errors_total")
	deliveryJournalBytes              = expvar.NewInt("agent_delivery_journal_bytes")
	deliveryUnacknowledgedBytes       = expvar.NewInt("agent_delivery_unacknowledged_bytes")
)

var boundedDeliveryLabels = map[string]struct{}{
	"prepared":              {},
	"accepted":              {},
	"dispatching":           {},
	"completed":             {},
	"failed":                {},
	"cancelled":             {},
	"interrupted_unknown":   {},
	"same_hash":             {},
	"hash_conflict":         {},
	"cursor_expired":        {},
	"sequence_conflict":     {},
	"owner_mismatch":        {},
	"journal_corrupt":       {},
	"journal_newer_version": {},
	"journal_full":          {},
	"stream_full":           {},
	"storage_unavailable":   {},
	"unknown":               {},
}

func RecordSubmission(outcome string) {
	deliverySubmissionsTotal.Add(boundedDeliveryLabel(outcome), 1)
}

func RecordDuplicateSubmission(result string) {
	deliveryDuplicateSubmissionsTotal.Add(boundedDeliveryLabel(result), 1)
}

func RecordReplayedEvents(count int) {
	if count > 0 {
		deliveryReplayedEventsTotal.Add(int64(count))
	}
}

func RecordSequenceError(reason string) {
	deliverySequenceErrorsTotal.Add(boundedDeliveryLabel(reason), 1)
}

func RecordUncertainSubmission(cause string) {
	deliveryUncertainSubmissionsTotal.Add(boundedDeliveryLabel(cause), 1)
}

func RecordJournalError(reason string) {
	deliveryJournalErrorsTotal.Add(boundedDeliveryLabel(reason), 1)
}

func SetJournalGauges(bytes, unacknowledgedBytes int64) {
	deliveryJournalBytes.Set(maxInt64(bytes, 0))
	deliveryUnacknowledgedBytes.Set(maxInt64(unacknowledgedBytes, 0))
}

func boundedDeliveryLabel(value string) string {
	value = strings.TrimSpace(value)
	if _, ok := boundedDeliveryLabels[value]; ok {
		return value
	}
	return "unknown"
}

func classifyJournalError(err error) string {
	switch {
	case err == nil:
		return "unknown"
	case errors.Is(err, ErrJournalCorrupt):
		return "journal_corrupt"
	case errors.Is(err, ErrJournalNewerVersion):
		return "journal_newer_version"
	case errors.Is(err, ErrJournalFull):
		return "journal_full"
	case errors.Is(err, ErrStreamFull):
		return "stream_full"
	case errors.Is(err, ErrSequenceConflict):
		return "sequence_conflict"
	case errors.Is(err, ErrOwnerMismatch):
		return "owner_mismatch"
	default:
		return "storage_unavailable"
	}
}

func maxInt64(value, lower int64) int64 {
	if value < lower {
		return lower
	}
	return value
}
