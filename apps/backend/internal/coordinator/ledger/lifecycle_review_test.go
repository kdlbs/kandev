package ledger

import "testing"

func TestStoppedLedgerDoesNotReadClosedRepositoryOnStart(t *testing.T) {
	f := newFixture(t)
	f.l.Start(t.Context())
	f.l.Stop()
	if err := f.l.deps.RO.Close(); err != nil {
		t.Fatal(err)
	}
	before := FailureCount(StageStart)
	f.l.Start(t.Context())
	f.l.Stop()
	if got := FailureCount(StageStart); got != before {
		t.Fatalf("stopped ledger accessed closed repository: start failures %d -> %d", before, got)
	}
}
