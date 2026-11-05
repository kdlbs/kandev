package models

// DeferredLaunchDiscardOutcome is the closed set of results an explicit user
// discard of a pending deferred launch can produce. The three cases are
// deliberately distinguishable: "there was nothing to discard" is not the same
// answer as "the record is gone" and neither is the same as "the record is
// still there", so a caller can never mistake an incomplete discard for a
// completed one.
type DeferredLaunchDiscardOutcome string

const (
	// DeferredLaunchDiscardAlreadyClear means the task holds no ceiling-deferred
	// launch, so nothing was written.
	DeferredLaunchDiscardAlreadyClear DeferredLaunchDiscardOutcome = "already_clear"
	// DeferredLaunchDiscardDiscarded means the ceiling half of the shared
	// deferred_launch record was removed, the launch's automation run was
	// settled, and its automation start ownership was released or never
	// belonged to this launch.
	DeferredLaunchDiscardDiscarded DeferredLaunchDiscardOutcome = "discarded"
	// DeferredLaunchDiscardRetained means the durable record, and with it the
	// launch ownership, is still in place because the discard could not
	// complete. Reason names the incomplete step.
	DeferredLaunchDiscardRetained DeferredLaunchDiscardOutcome = "retained"
)

// DeferredLaunchDiscardReason is the closed set of reasons carried by a
// retained outcome. It is empty for every completed outcome.
type DeferredLaunchDiscardReason string

const (
	// DeferredLaunchDiscardReasonClaimInFlight means another dispatcher (the
	// replay sweep or Send Now) currently owns the record and may be launching
	// it. The record and that ownership are left untouched.
	DeferredLaunchDiscardReasonClaimInFlight DeferredLaunchDiscardReason = "claim_in_flight"
	// DeferredLaunchDiscardReasonSettlement means the automation run the launch
	// serves could not be settled. Clearing the record first would leave an open
	// run with nothing to settle it.
	DeferredLaunchDiscardReasonSettlement DeferredLaunchDiscardReason = "settlement_incomplete"
	// DeferredLaunchDiscardReasonOwnership means this request had to release the
	// automation start ownership the launch carried and could not prove it did.
	// Leaving the record in place keeps the marker meaningful.
	DeferredLaunchDiscardReasonOwnership DeferredLaunchDiscardReason = "ownership_incomplete"
	// DeferredLaunchDiscardReasonClear means the compare-and-set clear could not
	// be proven to land.
	DeferredLaunchDiscardReasonClear DeferredLaunchDiscardReason = "clear_incomplete"
	// DeferredLaunchDiscardReasonRecordUnreadable means a record claims the
	// ceiling half but cannot be decoded, so its identity cannot be compared.
	// It is reported rather than silently discarded.
	DeferredLaunchDiscardReasonRecordUnreadable DeferredLaunchDiscardReason = "record_unreadable"
	// DeferredLaunchDiscardReasonRecordSuperseded means the record this request
	// observed was replaced or consumed before it could be claimed. Nothing was
	// settled, and the newer record is not this request's to discard.
	DeferredLaunchDiscardReasonRecordSuperseded DeferredLaunchDiscardReason = "record_superseded"
)

// DeferredLaunchDiscardResult is the answer to one explicit discard request.
type DeferredLaunchDiscardResult struct {
	Outcome DeferredLaunchDiscardOutcome
	Reason  DeferredLaunchDiscardReason
}
