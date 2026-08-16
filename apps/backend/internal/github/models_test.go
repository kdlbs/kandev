package github

import (
	"encoding/json"
	"testing"
)

// TestTaskPR_JSONMarshalsExplicitNullsForOutcomeFields is the AC-30
// regression: the frontend distinguishes "not observed" (explicit JSON
// null) from "observed absent" only if these five fields are always present
// in the payload. Adding `omitempty` to any of them would make an
// unobserved field disappear from the JSON entirely instead of serializing
// as null, and every other TaskPR test would stay green because none of
// them inspect the raw JSON — only this test would catch it.
func TestTaskPR_JSONMarshalsExplicitNullsForOutcomeFields(t *testing.T) {
	tp := &TaskPR{
		TaskID: "task-1", Owner: "owner", Repo: "repo", PRNumber: 1,
	}

	raw, err := json.Marshal(tp)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, key := range []string{
		"is_draft", "changed_files", "merged_by_login", "closed_by_login", "auto_merge_observed_at",
	} {
		value, present := decoded[key]
		if !present {
			t.Errorf("key %q missing from JSON payload entirely — want present with value null (an omitempty tag would do this)", key)
			continue
		}
		if string(value) != "null" {
			t.Errorf("key %q = %s, want null", key, value)
		}
	}
}
