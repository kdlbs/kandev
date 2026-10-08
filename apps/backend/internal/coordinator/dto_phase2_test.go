package coordinator

import (
	"context"
	"encoding/json"
	"testing"
)

func decodeMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestProposalDTO_Phase1ShapeHasNoPhase2Keys(t *testing.T) {
	_, c, _ := phase2Fixture(t, false)
	m := decodeMap(t, NewProposalDTOFor(&Proposal{CoordinatorID: c.ID, Spec: sampleSpec()}, false))
	for _, k := range []string{"kind", "target_task_id", "standing_order_ids", "starts_agent", "outcome"} {
		if _, ok := m[k]; ok {
			t.Fatalf("phase-1 DTO carries %q", k)
		}
	}
}

func TestProposalDTO_Phase2Fields(t *testing.T) {
	p := &Proposal{Kind: ProposalKindCreateTask, Spec: sampleSpec()}
	m := decodeMap(t, NewProposalDTOFor(p, true))
	if m["kind"] != ProposalKindCreateTask || m["starts_agent"] != false {
		t.Fatalf("m = %+v", m)
	}
	if ids, ok := m["standing_order_ids"].([]any); !ok || len(ids) != 0 {
		t.Fatalf("standing_order_ids = %#v, want []", m["standing_order_ids"])
	}
	if v, ok := m["outcome"]; !ok || v != nil {
		t.Fatalf("outcome = %#v, want null", v)
	}
	if _, ok := m["spec"].(map[string]any); !ok {
		t.Fatalf("spec = %#v", m["spec"])
	}
}

func TestProposalDTO_NonCreateKindCarriesRawSpecAndOutcome(t *testing.T) {
	outcome := `{"ok":true}`
	target := "task-9"
	p := &Proposal{Kind: "comment_on_task", RawSpec: `{"body":"hi"}`, TargetTaskID: &target, OutcomeJSON: &outcome}
	m := decodeMap(t, NewProposalDTOFor(p, true))
	if spec, _ := m["spec"].(map[string]any); spec["body"] != "hi" {
		t.Fatalf("spec = %#v", m["spec"])
	}
	if m["target_task_id"] != "task-9" {
		t.Fatalf("target_task_id = %#v", m["target_task_id"])
	}
	if o, _ := m["outcome"].(map[string]any); o["ok"] != true {
		t.Fatalf("outcome = %#v", m["outcome"])
	}
}

func TestCoordinatorDTO_PolicyFieldsFollowFlag(t *testing.T) {
	_, c, svc := phase2Fixture(t, true)
	h := &Handlers{service: svc}
	dto, err := h.coordinatorDTO(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	m := decodeMap(t, dto)
	watches, _ := m["watches"].(map[string]any)
	policy, _ := m["policy"].(map[string]any)
	if watches["scope"] != "all" || policy["actions"] == nil {
		t.Fatalf("m = %+v", m)
	}
	if ids, ok := watches["workflow_ids"].([]any); !ok || len(ids) != 0 {
		t.Fatalf("workflow_ids = %#v", watches["workflow_ids"])
	}
	if _, ok := m["policy_revision"]; !ok {
		t.Fatal("policy_revision missing")
	}

	svc.phase2 = false
	dto, err = h.coordinatorDTO(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	m = decodeMap(t, dto)
	for _, k := range []string{"policy", "policy_revision", "watches"} {
		if _, ok := m[k]; ok {
			t.Fatalf("phase-1 DTO carries %q", k)
		}
	}
}
