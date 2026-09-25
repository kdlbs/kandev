package models

import "strings"

// The deferred_launch record carries three independent, simultaneously-possible
// meanings discriminated by key rather than by position: a dependency-chain intent
// (DeferredLaunchStartWhenUnblockedKey), the WIP-overflow meaning (the record's
// presence with neither discriminator), and a session-ceiling deferral. A record may
// carry the chain intent and the ceiling deferral at once, so every predicate over
// this record tolerates both.
//
// Every key this card adds is prefixed ceiling_, and that prefix is load-bearing
// rather than cosmetic: it is how the ceiling's writes and the pre-existing WIP
// writes partition the shared record without either side keeping a list of the
// other's keys in sync.
const (
	// CeilingDeferredKey is the boolean discriminator marking a launch deferred
	// because the instance was at its session ceiling.
	CeilingDeferredKey = "ceiling_deferred"

	// CeilingQueuedAtKey records when the launch first entered the queue, as
	// RFC 3339 UTC. It is a diagnostic: nothing orders by it.
	CeilingQueuedAtKey = "ceiling_queued_at"

	// CeilingLaunchKindKey discriminates which launch shape the record replays.
	CeilingLaunchKindKey = "ceiling_launch_kind"

	// CeilingLaunchPayloadKey nests every replay field. It is redacted from the
	// public projection, because the payload carries the launch-scoped environment,
	// the composed prompt and its attachments.
	CeilingLaunchPayloadKey = "ceiling_launch_payload"

	// CeilingLaunchClaimKey is a short-lived compare-and-set claim held by the
	// replay or explicit Send Now dispatcher while it owns this exact deferred
	// launch. It is prefixed with ceiling_ so WIP and ceiling owners can clear
	// their own halves of the shared record independently.
	CeilingLaunchClaimKey = "ceiling_launch_claim"

	// CeilingLaunchClaimExpiresAtKey records the UTC lease deadline for an
	// in-flight ceiling launch claim. A process that exits while holding a
	// claim can therefore be recovered by a later dispatcher.
	CeilingLaunchClaimExpiresAtKey = "expires_at"

	// CeilingLaunchEntryBindingKey nests the exact workflow entry identity in
	// a workflow-origin replay payload.
	CeilingLaunchEntryBindingKey = "workflow_entry_binding"

	// CeilingLaunchOriginKey stores the automatic/manual classification so a
	// deferred automatic launch cannot be re-admitted later as a manual override.
	CeilingLaunchOriginKey = "ceiling_launch_origin"

	// CeilingReasonCodeKey carries the reason code a human reads on the card.
	CeilingReasonCodeKey = "ceiling_reason_code"

	// CeilingSurfaceWrittenAtKey is stamped only once the card note has been
	// written successfully, so a failed note is retried rather than suppressed.
	CeilingSurfaceWrittenAtKey = "ceiling_surface_written_at"

	// CeilingSurfaceAttemptCountKey bounds those retries. It lives on the record
	// rather than in process memory so a restart cannot reset the bound.
	CeilingSurfaceAttemptCountKey = "ceiling_surface_attempt_count"

	// CeilingPopulationAtRefusalKey records the population the admission
	// controller saw at refusal time, so a later retry's card note (AC-49g)
	// reproduces the same content rather than a freshly re-queried number. It
	// is absent, never zero, when the population was unknown at refusal
	// (AC-33a): an absent key is the "unknown" signal, not a rendered word.
	CeilingPopulationAtRefusalKey = "ceiling_population_at_refusal"

	// CeilingValueAtRefusalKey records the ceiling itself at refusal time. It
	// is always known, unlike the population.
	CeilingValueAtRefusalKey = "ceiling_value_at_refusal"

	ceilingRecordKeyPrefix = "ceiling_"
)

// IsCeilingRecordKey reports whether a key inside the deferred_launch record
// belongs to the session ceiling rather than to the pre-existing WIP meaning.
func IsCeilingRecordKey(key string) bool {
	return strings.HasPrefix(key, ceilingRecordKeyPrefix)
}

// HasCeilingDeferredIntent reports whether the task holds a launch deferred by the
// session ceiling.
func HasCeilingDeferredIntent(task *Task) bool {
	record, ok := deferredLaunchRecord(task)
	if !ok {
		return false
	}
	flag, ok := record[CeilingDeferredKey].(bool)
	return ok && flag
}

// deferredLaunchRecord reads the shared record as an object. Every existing reader
// already treats a non-object value as no intent, so this reports the same.
func deferredLaunchRecord(task *Task) (map[string]interface{}, bool) {
	if task == nil || task.Metadata == nil {
		return nil, false
	}
	record, ok := task.Metadata[MetaKeyDeferredLaunch].(map[string]interface{})
	return record, ok
}
