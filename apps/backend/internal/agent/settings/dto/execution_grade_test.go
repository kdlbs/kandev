package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExecutionGradeWireContract(t *testing.T) {
	for _, raw := range []string{`{}`, `{"execution_grade":null}`, `{"execution_grade":""}`, `{"execution_grade":"Tier-A"}`} {
		t.Run(raw, func(t *testing.T) {
			var req ProfileUpdateRequest
			if err := json.Unmarshal([]byte(raw), &req); err != nil {
				t.Fatal(err)
			}
			req.ID = "profile"
			if err := req.Validate(); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			_, present := got["execution_grade"]
			wantPresent := strings.Contains(raw, `""`) || strings.Contains(raw, `Tier-A`)
			if present != wantPresent {
				t.Fatalf("wire %s lost explicit grade: %s", raw, encoded)
			}
		})
	}
	for _, grade := range []string{" leading", "trailing ", "a\nb", strings.Repeat("界", 65)} {
		raw, _ := json.Marshal(map[string]any{"id": "profile", "agent_id": "agent", "name": "Profile", "execution_grade": grade})
		var patch ProfileUpdateRequest
		var create ProfileCreateRequest
		if err := json.Unmarshal(raw, &patch); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &create); err != nil {
			t.Fatal(err)
		}
		if patch.Validate() == nil || create.Validate() == nil {
			t.Errorf("invalid grade accepted: %q", grade)
		}
	}
	var wrong ProfileUpdateRequest
	if err := json.Unmarshal([]byte(`{"execution_grade":17}`), &wrong); err == nil {
		t.Error("non-string grade accepted")
	}
}

func TestExecutionGradeUnicodeBoundaryAndSchema(t *testing.T) {
	grade := strings.Repeat("界", 64)
	if err := (ProfileCreateRequest{AgentID: "agent", Name: "Profile", ExecutionGrade: grade}).Validate(); err != nil {
		t.Fatalf("64 Unicode characters rejected: %v", err)
	}
	for _, field := range ProfileContractFields() {
		if field.Path == "execution_grade" {
			if field.JSONType != "string" || field.Support != "read_write" {
				t.Fatalf("grade contract=%#v", field)
			}
			return
		}
	}
	t.Fatal("execution_grade absent from exported profile contract")
}
