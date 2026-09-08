package quorummetrics

import (
	"expvar"
	"strconv"
	"strings"
)

// officeReviewSeatEnsureTotal counts one ensure_participant_seat action
// outcome, labelled by role and outcome
// (office_review_seat_ensure_total{role,outcome},
// REQ-OFFICE-REVIEW-SEATS-004.1/-001.11).
var officeReviewSeatEnsureTotal = expvar.NewMap("office_review_seat_ensure_total")

// officeReviewSeatProvenanceTotal counts one seat's casting provenance,
// labelled by role, provenance and self_review
// (office_review_seat_provenance_total{role,provenance,self_review},
// REQ-OFFICE-REVIEW-SEATS-004.4/-004.5).
var officeReviewSeatProvenanceTotal = expvar.NewMap("office_review_seat_provenance_total")

// seatMetricLabel builds a "k1=v1;k2=v2;..." label string for an expvar
// map key, mirroring internal/office/scheduler's metricLabel convention so
// a future Prometheus translation layer can split on `;` and `=` without
// re-shaping storage. Keys are not escaped — callers pass only bounded,
// alphanumeric dimension values (AC-OFFICE-REVIEW-SEATS-004.6).
func seatMetricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// RecordSeatEnsureOutcome counts one ensure_participant_seat outcome for
// role. outcome is one of: seated, already_seated, no_candidate, no_runner,
// error, malformed_config.
func RecordSeatEnsureOutcome(role, outcome string) {
	officeReviewSeatEnsureTotal.Add(seatMetricLabel("role", role, "outcome", outcome), 1)
}

// RecordSeatProvenance counts one seat's casting provenance and whether it
// is a self-review.
func RecordSeatProvenance(role, provenance string, selfReview bool) {
	officeReviewSeatProvenanceTotal.Add(
		seatMetricLabel("role", role, "provenance", provenance, "self_review", strconv.FormatBool(selfReview)), 1)
}
