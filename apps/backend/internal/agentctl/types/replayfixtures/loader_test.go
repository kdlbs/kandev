package replayfixtures

import "testing"

// TestLoadCorpusIsValid pins that the embedded fixture corpus loads cleanly:
// every structural rule in provider-error-recovery-02.md#fixture-document
// passes, every required (gateway, case) cell is present, and no two
// fixtures in the same case carry equivalent frames.
func TestLoadCorpusIsValid(t *testing.T) {
	fixtures, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(fixtures) != 22 {
		t.Fatalf("len(fixtures) = %d, want 22", len(fixtures))
	}
	if missing := MissingRequiredCells(fixtures); len(missing) != 0 {
		t.Fatalf("missing required cells: %v", missing)
	}
}

func TestMissingRequiredCellsOnEmptyCorpus(t *testing.T) {
	missing := MissingRequiredCells(nil)
	// 4 gateways * 4 always-required cases + 2 classifying gateways * 3
	// prose- cases = 16 + 6 = 22.
	if len(missing) != 22 {
		t.Fatalf("len(missing) = %d, want 22 for an empty corpus", len(missing))
	}
}
