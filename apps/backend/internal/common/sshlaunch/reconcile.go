package sshlaunch

import "fmt"

// ReconcileMode selects the helper operation. The zero value is terminate for
// compatibility with a request encoded before this field existed.
type ReconcileMode string

const (
	// ReconcileModeTerminate verifies the recorded controller, requires its
	// disposable session path to be absent, sends SIGTERM through the retained
	// pidfd, and proves its session children drained.
	ReconcileModeTerminate ReconcileMode = "terminate"
	// ReconcileModeVerify checks the same launch identity against the live
	// process without signalling it or removing any path. A caller uses it to
	// authorize its own guarded stop of a receipt-bearing record.
	ReconcileModeVerify ReconcileMode = "verify"
)

// EffectiveMode returns the mode this request selects.
func (m ReconcileMode) Effective() ReconcileMode {
	if m == "" {
		return ReconcileModeTerminate
	}
	return m
}

func (m ReconcileMode) Validate() error {
	switch m.Effective() {
	case ReconcileModeTerminate, ReconcileModeVerify:
		return nil
	default:
		return fmt.Errorf("%w: unknown reconcile mode %q", ErrIncompleteReceipt, m)
	}
}

// ReconcileRequest is the private wire request sent to `agentctl ssh-reconcile`
// over SSH stdin. It carries the recorded launch proof and the operation the
// caller needs; the helper derives every live fact itself, so a caller cannot
// widen authority by adding fields.
type ReconcileRequest struct {
	Version int           `json:"version"`
	Mode    ReconcileMode `json:"mode,omitempty"`
	Receipt Receipt       `json:"receipt"`
}

func (r ReconcileRequest) Validate() error {
	if r.Version != Version {
		return fmt.Errorf("%w: unsupported reconcile request version %d", ErrIncompleteReceipt, r.Version)
	}
	if err := r.Mode.Validate(); err != nil {
		return err
	}
	return r.Receipt.Validate()
}

// ReconcileOutcome is the closed set of results a remote reconciliation can
// report. Every value other than OutcomeStopped, OutcomeAbsent, or the
// verify-only OutcomeVerified leaves the recorded owner unresolved.
type ReconcileOutcome string

const (
	// OutcomeStopped means the recorded original process was verified, exited
	// after SIGTERM, and no session child survived it.
	OutcomeStopped ReconcileOutcome = "stopped"
	// OutcomeAbsent means the recorded process was positively gone before any
	// signal was sent and no session child survived it.
	OutcomeAbsent ReconcileOutcome = "absent"
	// OutcomeVerified means the live process still matches the recorded launch
	// identity. Verify mode never signals, removes, or settles anything.
	OutcomeVerified ReconcileOutcome = "verified"
	// OutcomeUnresolved means the helper could not prove safe progress. Reason
	// names which evidence failed.
	OutcomeUnresolved ReconcileOutcome = "unresolved"
)

// ReconcileReason is the closed set of unresolved reasons. They are stable
// diagnostic codes, never free-form messages.
type ReconcileReason string

const (
	ReasonIdentityConflict       ReconcileReason = "identity_conflict"
	ReasonLaunchIdentityMissing  ReconcileReason = "launch_identity_missing"
	ReasonUnsupportedIdentity    ReconcileReason = "unsupported_identity"
	ReasonProbeUnknown           ReconcileReason = "probe_unknown"
	ReasonTerminationUnconfirmed ReconcileReason = "termination_unconfirmed"
)

// ReconcileResult is the bounded wire response. Detail is a short, stable
// explanation that never includes credentials or raw procfs contents.
type ReconcileResult struct {
	Outcome ReconcileOutcome `json:"outcome"`
	Reason  ReconcileReason  `json:"reason,omitempty"`
	Detail  string           `json:"detail,omitempty"`
}

func (r ReconcileResult) Validate() error {
	switch r.Outcome {
	case OutcomeStopped, OutcomeAbsent, OutcomeVerified:
		if r.Reason != "" {
			return fmt.Errorf("reconcile result %q must not carry a reason", r.Outcome)
		}
	case OutcomeUnresolved:
		switch r.Reason {
		case ReasonIdentityConflict, ReasonLaunchIdentityMissing, ReasonUnsupportedIdentity,
			ReasonProbeUnknown, ReasonTerminationUnconfirmed:
		default:
			return fmt.Errorf("reconcile result has an unknown unresolved reason %q", r.Reason)
		}
	default:
		return fmt.Errorf("reconcile result has an unknown outcome %q", r.Outcome)
	}
	return nil
}

// Terminal reports whether the result settles the recorded owner for cleanup.
// A verify result never settles anything.
func (r ReconcileResult) Terminal() bool {
	return r.Outcome == OutcomeStopped || r.Outcome == OutcomeAbsent
}

// Unresolved builds an unresolved result with a closed reason.
func Unresolved(reason ReconcileReason, detail string) ReconcileResult {
	return ReconcileResult{Outcome: OutcomeUnresolved, Reason: reason, Detail: detail}
}
