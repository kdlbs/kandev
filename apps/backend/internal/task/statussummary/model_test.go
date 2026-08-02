package statussummary

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTaskStatusSummarySemanticEqualityIgnoresTransportMetadata(t *testing.T) {
	first := TaskStatusSummary{
		Revision:  1,
		UpdatedAt: time.Now().UTC(),
		PrimarySession: &PrimarySessionSummary{
			ID: "session-1", State: "WAITING_FOR_INPUT",
		},
		ActiveError: &ActiveErrorSummary{SessionID: "session-1", Stamp: "error-1", Preview: "failed"},
	}
	second := first
	second.Revision = 9
	second.UpdatedAt = first.UpdatedAt.Add(time.Hour)
	if !first.SemanticEqual(second) {
		t.Fatal("revision and updated_at should not affect semantic equality")
	}

	second.PrimarySession = &PrimarySessionSummary{ID: "session-1", State: "RUNNING"}
	if first.SemanticEqual(second) {
		t.Fatal("observed status changes must not compare equal")
	}
}

func TestTaskStatusSummarySemanticJSONIsBoundedAndOmitsTransportMetadata(t *testing.T) {
	summary := TaskStatusSummary{
		Revision:  4,
		UpdatedAt: time.Now().UTC(),
		ActiveError: &ActiveErrorSummary{
			SessionID: "session-1",
			Stamp:     "error-1",
			Preview:   "safe preview",
		},
		Git: &GitSummary{ChangedFiles: 2, Additions: 3},
	}
	payload, err := summary.SemanticJSON()
	if err != nil {
		t.Fatalf("semantic JSON: %v", err)
	}
	if strings.Contains(string(payload), `"revision"`) || strings.Contains(string(payload), `"updated_at"`) {
		t.Fatalf("semantic payload contains transport metadata: %s", payload)
	}
	var decoded TaskStatusSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode semantic JSON: %v", err)
	}
	if !summary.SemanticEqual(decoded) {
		t.Fatalf("semantic round trip changed value: %#v", decoded)
	}
}

func TestTaskStatusSummaryValidateBoundsErrorPreview(t *testing.T) {
	valid := TaskStatusSummary{ActiveError: &ActiveErrorSummary{Preview: strings.Repeat("é", MaxActiveErrorPreviewBytes/2)}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid UTF-8 preview rejected: %v", err)
	}

	tooLarge := valid
	tooLarge.ActiveError = &ActiveErrorSummary{Preview: strings.Repeat("x", MaxActiveErrorPreviewBytes+1)}
	if err := tooLarge.Validate(); err == nil {
		t.Fatal("oversized preview should be rejected")
	}

	invalidUTF8 := TaskStatusSummary{ActiveError: &ActiveErrorSummary{Preview: string([]byte{0xff})}}
	if err := invalidUTF8.Validate(); err == nil {
		t.Fatal("invalid UTF-8 preview should be rejected")
	}
}
