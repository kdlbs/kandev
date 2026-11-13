package streams

// Context compaction observation kinds.
const (
	ContextCompactionObservationKindCompletion = "completion"
	ContextCompactionObservationKindReporting  = "reporting"
)

// Context compaction observation sources.
const (
	ContextCompactionSourceUsageDrop     = "usage_drop"
	ContextCompactionSourceCopilotNative = "copilot_native"
)

// Context compaction reporting availability values.
const (
	ContextCompactionAvailabilityAvailable   = "available"
	ContextCompactionAvailabilityUnavailable = "unavailable"
)

// Context compaction reporting coverage values.
const (
	ContextCompactionCoverageObserved = "observed"
	ContextCompactionCoveragePartial  = "partial"
)

// ContextCompactionObservation is the bounded provider evidence carried by a
// context_compaction_observation event. Completion observations require an
// attempt identity and an explicit applied success. Reporting observations
// describe collection availability and coverage without asserting a count.
type ContextCompactionObservation struct {
	Kind            string `json:"kind"`
	Provider        string `json:"provider"`
	NativeSessionID string `json:"native_session_id"`
	AttemptID       string `json:"attempt_id,omitempty"`
	Success         bool   `json:"success,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	Trigger         string `json:"trigger,omitempty"`
	Availability    string `json:"availability,omitempty"`
	Coverage        string `json:"coverage,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

// ValidCompletion reports whether the observation is a countable applied
// parent completion with a stable attempt identity.
func (o *ContextCompactionObservation) ValidCompletion() bool {
	return o != nil &&
		o.Kind == ContextCompactionObservationKindCompletion &&
		o.Provider != "" &&
		o.NativeSessionID != "" &&
		o.AttemptID != "" &&
		o.Success &&
		o.Outcome == "applied"
}

// ValidReporting reports whether the observation carries a supported
// availability or coverage update.
func (o *ContextCompactionObservation) ValidReporting() bool {
	if o == nil || o.Kind != ContextCompactionObservationKindReporting || o.Provider == "" {
		return false
	}
	if o.Availability != "" && o.Availability != ContextCompactionAvailabilityAvailable &&
		o.Availability != ContextCompactionAvailabilityUnavailable {
		return false
	}
	if o.Coverage != "" && o.Coverage != ContextCompactionCoverageObserved &&
		o.Coverage != ContextCompactionCoveragePartial {
		return false
	}
	return o.Availability != "" || o.Coverage != ""
}

// ContextCompactionReporting is the persisted reporting state surfaced next to
// the lifetime count. Missing metadata keeps legacy inferred-count behavior.
type ContextCompactionReporting struct {
	Source       string `json:"source"`
	Availability string `json:"availability"`
	Coverage     string `json:"coverage"`
	Revision     int64  `json:"revision"`
}

// NormalizeReporting returns a bounded reporting value or nil when unsupported.
func NormalizeReporting(reporting *ContextCompactionReporting) *ContextCompactionReporting {
	if reporting == nil {
		return nil
	}
	source := reporting.Source
	if source != ContextCompactionSourceUsageDrop && source != ContextCompactionSourceCopilotNative {
		return nil
	}
	availability := reporting.Availability
	if availability != ContextCompactionAvailabilityAvailable &&
		availability != ContextCompactionAvailabilityUnavailable {
		return nil
	}
	coverage := reporting.Coverage
	if coverage != ContextCompactionCoverageObserved && coverage != ContextCompactionCoveragePartial {
		return nil
	}
	if reporting.Revision < 0 {
		return nil
	}
	return &ContextCompactionReporting{
		Source:       source,
		Availability: availability,
		Coverage:     coverage,
		Revision:     reporting.Revision,
	}
}
