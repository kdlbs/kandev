package quorummetrics

import (
	"expvar"
	"testing"
)

func TestSeatMetricLabel(t *testing.T) {
	got := seatMetricLabel("role", "reviewer", "outcome", "seated")
	want := "role=reviewer;outcome=seated"
	if got != want {
		t.Fatalf("label = %q, want %q", got, want)
	}
}

func TestSeatMetricLabel_OddPairsReturnsEmpty(t *testing.T) {
	if got := seatMetricLabel("role"); got != "" {
		t.Fatalf("label = %q, want empty for an odd argument count", got)
	}
}

func TestRecordSeatEnsureOutcome(t *testing.T) {
	key := seatMetricLabel("role", "seat-ensure-role", "outcome", "seated")
	before := readCounter(t, officeReviewSeatEnsureTotal, key)
	RecordSeatEnsureOutcome("seat-ensure-role", "seated")
	after := readCounter(t, officeReviewSeatEnsureTotal, key)
	if after-before != 1 {
		t.Errorf("counter delta = %d, want 1", after-before)
	}
}

func TestRecordSeatEnsureOutcome_DistinctPerOutcome(t *testing.T) {
	seatedKey := seatMetricLabel("role", "seat-ensure-outcome-role", "outcome", "seated")
	errorKey := seatMetricLabel("role", "seat-ensure-outcome-role", "outcome", "error")
	beforeSeated := readCounter(t, officeReviewSeatEnsureTotal, seatedKey)
	beforeError := readCounter(t, officeReviewSeatEnsureTotal, errorKey)

	RecordSeatEnsureOutcome("seat-ensure-outcome-role", "seated")

	afterSeated := readCounter(t, officeReviewSeatEnsureTotal, seatedKey)
	afterError := readCounter(t, officeReviewSeatEnsureTotal, errorKey)
	if afterSeated-beforeSeated != 1 {
		t.Errorf("seated counter delta = %d, want 1", afterSeated-beforeSeated)
	}
	if afterError != beforeError {
		t.Errorf("error-outcome counter changed from a seated increment: before=%d after=%d", beforeError, afterError)
	}
}

func TestRecordSeatProvenance(t *testing.T) {
	key := seatMetricLabel("role", "seat-provenance-role", "provenance", "eligible_pool", "self_review", "false")
	before := readCounter(t, officeReviewSeatProvenanceTotal, key)
	RecordSeatProvenance("seat-provenance-role", "eligible_pool", false)
	after := readCounter(t, officeReviewSeatProvenanceTotal, key)
	if after-before != 1 {
		t.Errorf("counter delta = %d, want 1", after-before)
	}
}

func TestRecordSeatProvenance_SelfReviewIsADistinctLabel(t *testing.T) {
	notSelfKey := seatMetricLabel("role", "seat-provenance-selfreview-role", "provenance", "runner_fallback", "self_review", "false")
	selfKey := seatMetricLabel("role", "seat-provenance-selfreview-role", "provenance", "runner_fallback", "self_review", "true")
	beforeNotSelf := readCounter(t, officeReviewSeatProvenanceTotal, notSelfKey)
	beforeSelf := readCounter(t, officeReviewSeatProvenanceTotal, selfKey)

	RecordSeatProvenance("seat-provenance-selfreview-role", "runner_fallback", true)

	afterNotSelf := readCounter(t, officeReviewSeatProvenanceTotal, notSelfKey)
	afterSelf := readCounter(t, officeReviewSeatProvenanceTotal, selfKey)
	if afterSelf-beforeSelf != 1 {
		t.Errorf("self_review=true counter delta = %d, want 1", afterSelf-beforeSelf)
	}
	if afterNotSelf != beforeNotSelf {
		t.Errorf("self_review=false counter changed from a self_review=true increment: before=%d after=%d", beforeNotSelf, afterNotSelf)
	}
}

func TestOfficeReviewSeatCountersPublishedAtKnownNames(t *testing.T) {
	if expvar.Get("office_review_seat_ensure_total") == nil {
		t.Error("expvar \"office_review_seat_ensure_total\" not published — /debug/vars consumers will miss it")
	}
	if expvar.Get("office_review_seat_provenance_total") == nil {
		t.Error("expvar \"office_review_seat_provenance_total\" not published — /debug/vars consumers will miss it")
	}
}
