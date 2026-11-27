package dynamic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

var errIncompleteUnclassifiedDiagnostic = errors.New("unclassified diagnostic is incomplete")

type unclassifiedFingerprintInput struct {
	Version    int                       `json:"version"`
	Code       routingerr.Code           `json:"code"`
	Origin     UnclassifiedFailureOrigin `json:"origin"`
	Phase      routingerr.Phase          `json:"phase"`
	ProviderID string                    `json:"provider_id"`
	Diagnostic string                    `json:"diagnostic"`
}

// emptyTurnDiagnostic stands in for the diagnostic of an empty end_turn
// completion. It is a constant, so consecutive empty turns share one identity
// and no transcript or provider text is hashed.
const emptyTurnDiagnostic = "end_turn without output"

func unclassifiedFailureFingerprint(evidence UnclassifiedFailureEvidence, failure *routingerr.Error) (string, error) {
	if evidence.Origin == UnclassifiedOriginEmptyTurn {
		return hashUnclassifiedFingerprint(unclassifiedFingerprintInput{
			Version: 1, Code: failure.Code, Origin: evidence.Origin,
			Phase: evidence.Phase, Diagnostic: emptyTurnDiagnostic,
		})
	}

	diagnostic := evidence.DiagnosticText
	if !evidence.DiagnosticComplete || diagnostic == "" || !utf8.ValidString(diagnostic) ||
		len([]byte(diagnostic)) > 1024 || routingerr.SanitizeFullUnbounded(diagnostic) != diagnostic {
		return "", errIncompleteUnclassifiedDiagnostic
	}
	diagnostic = strings.Join(strings.Fields(diagnostic), " ")
	if diagnostic == "" || strings.TrimSpace(evidence.ProviderID) == "" {
		return "", errIncompleteUnclassifiedDiagnostic
	}
	return hashUnclassifiedFingerprint(unclassifiedFingerprintInput{
		Version: 1, Code: failure.Code, Origin: evidence.Origin,
		Phase: evidence.Phase, ProviderID: evidence.ProviderID, Diagnostic: diagnostic,
	})
}

func hashUnclassifiedFingerprint(input unclassifiedFingerprintInput) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
