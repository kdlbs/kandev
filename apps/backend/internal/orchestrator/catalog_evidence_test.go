package orchestrator

import "testing"

func TestClassifyCatalogEvidenceRequiresExplicitSameAttemptClientObservation(t *testing.T) {
	zero := true
	base := CatalogEvidenceInput{AttemptID: "attempt", ServedAttemptID: "attempt", ServedToolNames: []string{"message_task_kandev"}}
	if got := ClassifyCatalogEvidence(base); got != CatalogEvidenceUnknown {
		t.Fatalf("got %q", got)
	}
	base.ClientExactSearchZero = &zero
	if got := ClassifyCatalogEvidence(base); got != CatalogEvidenceClientPresentation {
		t.Fatalf("got %q", got)
	}
	base.ServedAttemptID = "other"
	if got := ClassifyCatalogEvidence(base); got != CatalogEvidenceUnknown {
		t.Fatalf("got %q", got)
	}
	base.ServedAttemptID = "attempt"
	base.ServedToolNames = nil
	if got := ClassifyCatalogEvidence(base); got != CatalogEvidenceBackendCatalog {
		t.Fatalf("got %q", got)
	}
}
