package changes

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

func encodeTurnChangeProvenance(provenance *turnchanges.Provenance) string {
	if !provenance.Valid() {
		return ""
	}
	encoded, err := json.Marshal(provenance)
	if err != nil || len(encoded) > turnchanges.MaxProvenanceBytes {
		return ""
	}
	return string(encoded)
}

func decodeTurnChangeProvenance(raw string) *turnchanges.Provenance {
	if raw == "" {
		return nil
	}
	provenance := &turnchanges.Provenance{}
	if err := json.Unmarshal([]byte(raw), provenance); err != nil || !provenance.Valid() {
		return nil
	}
	return provenance
}

func turnChangeComparisonVersion(row models.TurnRepositoryChangeSet) int {
	start := decodeTurnChangeProvenance(row.StartProvenanceJSON)
	end := decodeTurnChangeProvenance(row.EndProvenanceJSON)
	if start == nil || end == nil {
		return turnchanges.ComparisonVersionLegacy
	}
	return turnchanges.ComparisonVersion2
}

func turnChangeComparisonExpectation(
	comparison *turnchanges.CheckpointComparison,
	requestedVersion int,
) (models.TurnRepositoryComparison, models.TurnChangeReason, bool) {
	legacy := models.TurnRepositoryComparison{Version: turnchanges.ComparisonVersionLegacy, Basis: turnchanges.ComparisonLegacyRaw}
	if comparison == nil || comparison.ComparisonVersion != turnchanges.ComparisonVersion2 {
		if requestedVersion == turnchanges.ComparisonVersionLegacy {
			return legacy, "", true
		}
		// A retained older agentctl ignores version negotiation and returns the
		// raw pair. Keep that behavior explicitly legacy instead of looping.
		return legacy, "", true
	}
	if !comparison.ComparisonBasis.Valid() || comparison.ComparisonBasis == turnchanges.ComparisonLegacyRaw {
		return legacy, "", true
	}
	expectation := models.TurnRepositoryComparison{
		Version: int64(comparison.ComparisonVersion), Basis: comparison.ComparisonBasis,
		Reason:                  comparison.ComparisonReason,
		EffectiveStartCommitOID: comparison.EffectiveStartCommitOID,
		EffectiveStartTreeOID:   comparison.EffectiveStartTreeOID,
		EffectiveEndCommitOID:   comparison.EffectiveEndCommitOID,
		EffectiveEndTreeOID:     comparison.EffectiveEndTreeOID,
		ComparisonRef:           comparison.ComparisonRef,
	}
	if comparison.ComparisonBasis == turnchanges.ComparisonHistoryUnavailable {
		if expectation.Reason == "" {
			expectation.Reason = models.TurnChangeReasonHistoryProvenanceMissing
		}
		return expectation, expectation.Reason, true
	}
	if compareEndpointsMissing(expectation) {
		expectation.Basis = turnchanges.ComparisonHistoryUnavailable
		expectation.Reason = models.TurnChangeReasonHistoryProvenanceMissing
		return expectation, expectation.Reason, true
	}
	return expectation, "", true
}

func compareEndpointsMissing(comparison models.TurnRepositoryComparison) bool {
	return comparison.EffectiveStartCommitOID == "" || comparison.EffectiveStartTreeOID == "" ||
		comparison.EffectiveEndCommitOID == "" || comparison.EffectiveEndTreeOID == ""
}
