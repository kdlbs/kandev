package replayfixtures

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// forbiddenShape is one named pattern in a sanitisation rule. name appears in
// the load error so a violation is diagnosable without re-deriving which
// pattern matched.
type forbiddenShape struct {
	name    string
	pattern *regexp.Regexp
}

// payloadForbiddenShapes is the enforced definition of the "credential,
// bearer token, account or organization identifier" prohibition over a
// fixture's replayed payload (identity, frames and expect) —
// provider-error-recovery-02.md#fixture-document, "Sanitisation, scoped".
// This list is the definition, not a sample of it: extending it is a
// catalogue revision, not a design change.
var payloadForbiddenShapes = []forbiddenShape{
	{"Bearer token", regexp.MustCompile(`(?i)\bBearer\s+\S+`)},
	{"vendor key prefix", regexp.MustCompile(`(?i)\b(?:sk|sess|org)-[A-Za-z0-9_-]+`)},
	{"OpenCode identifier", regexp.MustCompile(`(?i)\b(?:ses|wrk)_[A-Za-z0-9_-]+`)},
	{"URL", regexp.MustCompile(`(?i)\bhttps?://\S+`)},
	{"absolute host path", regexp.MustCompile(`(?i)/(?:Users|home|var)/\S+`)},
	{"email address", regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)},
	{"RFC 4122 UUID", regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)},
}

// provenanceForbiddenShapes is the narrower rule over capture/source/capturedAt:
// the same credential and identifier shapes minus the blanket URL prohibition
// (a reconstructed fixture must cite a published contract, whose natural form
// is a URL), plus the private-host check the payload rule does not need,
// since no envelope needs a URL to replay faithfully.
var provenanceForbiddenShapes = []forbiddenShape{
	{"Bearer token", regexp.MustCompile(`(?i)\bBearer\s+\S+`)},
	{"vendor key prefix", regexp.MustCompile(`(?i)\b(?:sk|sess|org)-[A-Za-z0-9_-]+`)},
	{"OpenCode identifier", regexp.MustCompile(`(?i)\b(?:ses|wrk)_[A-Za-z0-9_-]+`)},
	{"email address", regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)},
	{"RFC 4122 UUID", regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)},
	{"URL userinfo component", regexp.MustCompile(`(?i)://[^/\s@]+@`)},
	{"private host", regexp.MustCompile(`(?i)\b(?:localhost|127\.0\.0\.1|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3})\b`)},
	{"internal or local host", regexp.MustCompile(`(?i)\b[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.(?:internal|local)\b`)},
}

func scanForbiddenShapes(text string, shapes []forbiddenShape) []string {
	var hits []string
	for _, shape := range shapes {
		if shape.pattern.MatchString(text) {
			hits = append(hits, shape.name)
		}
	}
	return hits
}

// validateSanitisation enforces provider-error-recovery-02.md's two
// sanitisation rules. The payload rule scans identity, frames and expect —
// the full replayed payload, including the corpus-unique session and
// execution identifiers a fixture must declare — against
// payloadForbiddenShapes, so a real captured identifier shape (a UUID, a
// vendor session prefix) is caught the same as one pasted into frame text.
// The provenance rule scans capture, source and capturedAt against the
// narrower provenanceForbiddenShapes, which permits a bare URL so a
// reconstructed fixture can cite its published contract.
func validateSanitisation(f Fixture) error {
	payload, err := json.Marshal(struct {
		Identity Identity `json:"identity"`
		Frames   []Frame  `json:"frames"`
		Expect   Expect   `json:"expect"`
	}{f.Identity, f.Frames, f.Expect})
	if err != nil {
		return fmt.Errorf("marshal payload for sanitisation: %w", err)
	}
	if hits := scanForbiddenShapes(string(payload), payloadForbiddenShapes); len(hits) > 0 {
		return fmt.Errorf("payload sanitisation: forbidden shape(s) %v", hits)
	}

	provenance := string(f.Capture) + " " + f.Source + " " + f.CapturedAt
	if hits := scanForbiddenShapes(provenance, provenanceForbiddenShapes); len(hits) > 0 {
		return fmt.Errorf("provenance sanitisation: forbidden shape(s) %v", hits)
	}
	return nil
}
