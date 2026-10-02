package coordinator

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewErrorResponseHasNoField(t *testing.T) {
	raw, err := json.Marshal(NewErrorResponse("not found"))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if string(raw) != `{"error":"not found"}` {
		t.Errorf("Marshal() = %s, want a bare error body with no field key", raw)
	}
}

func TestNewFieldErrorResponseIncludesField(t *testing.T) {
	raw, err := json.Marshal(NewFieldErrorResponse(&FieldError{Field: "name", Message: "name must be 1 to 60 characters"}))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if string(raw) != `{"error":"name must be 1 to 60 characters","field":"name"}` {
		t.Errorf("Marshal() = %s, want the error and field keys", raw)
	}
}

func TestCoordinatorDTOBaseFields(t *testing.T) {
	created := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	updated := created.Add(time.Hour)
	taskID := "task-1"
	c := &Coordinator{
		ID: "co-1", WorkspaceID: "ws-1", Name: "Release Coordinator",
		AgentProfileID: "ap-1", ExecutorProfileID: "ep-1", Context: "standing context",
		ConversationTaskID: &taskID, CreatedAt: created, UpdatedAt: updated,
	}
	dto := NewCoordinatorDTO(c)

	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	wantKeys := []string{"id", "workspace_id", "name", "agent_profile_id", "executor_profile_id", "context", "conversation_task_id", "created_at", "updated_at"}
	for _, key := range wantKeys {
		if _, ok := decoded[key]; !ok {
			t.Errorf("marshaled DTO missing key %q: %s", key, raw)
		}
	}
	for _, absent := range []string{"open_proposals", "agent_profile_status", "executor_profile_status"} {
		if _, ok := decoded[absent]; ok {
			t.Errorf("marshaled base DTO unexpectedly has key %q: %s", absent, raw)
		}
	}
	if decoded["conversation_task_id"] != taskID {
		t.Errorf("conversation_task_id = %v, want %q", decoded["conversation_task_id"], taskID)
	}
}

func TestCoordinatorDTONilConversationTaskIDIsExplicitNull(t *testing.T) {
	c := &Coordinator{ID: "co-1", WorkspaceID: "ws-1"}
	raw, err := json.Marshal(NewCoordinatorDTO(c))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	value, ok := decoded["conversation_task_id"]
	if !ok {
		t.Fatalf("marshaled DTO missing key %q: %s", "conversation_task_id", raw)
	}
	if value != nil {
		t.Errorf("conversation_task_id = %v, want null", value)
	}
}

func TestCoordinatorDTOWithOpenProposalsIncludesZero(t *testing.T) {
	dto := NewCoordinatorDTO(&Coordinator{ID: "co-1"}).WithOpenProposals(0)
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	value, ok := decoded["open_proposals"]
	if !ok {
		t.Fatalf("marshaled DTO missing key %q for a zero count: %s", "open_proposals", raw)
	}
	if value != float64(0) {
		t.Errorf("open_proposals = %v, want 0", value)
	}
}

func TestCoordinatorDTOWithProfileStatuses(t *testing.T) {
	dto := NewCoordinatorDTO(&Coordinator{ID: "co-1"}).WithProfileStatuses(ProfileStatusPassthrough, ProfileStatusMissing)
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if decoded["agent_profile_status"] != string(ProfileStatusPassthrough) {
		t.Errorf("agent_profile_status = %v, want %q", decoded["agent_profile_status"], ProfileStatusPassthrough)
	}
	if decoded["executor_profile_status"] != string(ProfileStatusMissing) {
		t.Errorf("executor_profile_status = %v, want %q", decoded["executor_profile_status"], ProfileStatusMissing)
	}
}

func TestNewCoordinatorListResponseNeverSerializesNull(t *testing.T) {
	raw, err := json.Marshal(NewCoordinatorListResponse(nil))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if string(raw) != `{"coordinators":[]}` {
		t.Errorf("Marshal(empty list) = %s, want {\"coordinators\":[]}", raw)
	}
}

func TestNewCoordinatorListResponseWrapsItems(t *testing.T) {
	items := []*CoordinatorDTO{NewCoordinatorDTO(&Coordinator{ID: "co-1"}).WithOpenProposals(2)}
	raw, err := json.Marshal(NewCoordinatorListResponse(items))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded struct {
		Coordinators []map[string]any `json:"coordinators"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if len(decoded.Coordinators) != 1 {
		t.Fatalf("len(Coordinators) = %d, want 1", len(decoded.Coordinators))
	}
	if decoded.Coordinators[0]["open_proposals"] != float64(2) {
		t.Errorf("open_proposals = %v, want 2", decoded.Coordinators[0]["open_proposals"])
	}
}

func TestPatchCoordinatorRequestStringFieldAbsent(t *testing.T) {
	req := PatchCoordinatorRequest{}
	value, present, err := req.StringField(PatchFieldName)
	if err != nil {
		t.Fatalf("StringField() unexpected error: %v", err)
	}
	if present {
		t.Errorf("present = true, want false for an absent field")
	}
	if value != nil {
		t.Errorf("value = %v, want nil for an absent field", value)
	}
}

func TestPatchCoordinatorRequestStringFieldNull(t *testing.T) {
	var req PatchCoordinatorRequest
	if err := json.Unmarshal([]byte(`{"name": null}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	value, present, err := req.StringField(PatchFieldName)
	if !present {
		t.Errorf("present = false, want true for a field sent as null")
	}
	if value != nil {
		t.Errorf("value = %v, want nil for a field sent as null", value)
	}
	var fieldErr *FieldError
	if err == nil {
		t.Fatal("StringField() error = nil, want a *FieldError for JSON null")
	}
	if !errors.As(err, &fieldErr) {
		t.Fatalf("StringField() error type = %T, want *FieldError", err)
	}
	if fieldErr.Field != PatchFieldName {
		t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, PatchFieldName)
	}
}

func TestPatchCoordinatorRequestStringFieldValue(t *testing.T) {
	var req PatchCoordinatorRequest
	if err := json.Unmarshal([]byte(`{"name": "New Name"}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	value, present, err := req.StringField(PatchFieldName)
	if err != nil {
		t.Fatalf("StringField() unexpected error: %v", err)
	}
	if !present {
		t.Errorf("present = false, want true for a sent field")
	}
	if value == nil || *value != "New Name" {
		t.Errorf("value = %v, want %q", value, "New Name")
	}
}

func TestPatchCoordinatorRequestStringFieldWrongType(t *testing.T) {
	var req PatchCoordinatorRequest
	if err := json.Unmarshal([]byte(`{"name": 42}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	_, _, err := req.StringField(PatchFieldName)
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("StringField() error type = %T, want *FieldError", err)
	}
	if fieldErr.Field != PatchFieldName {
		t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, PatchFieldName)
	}
}

func TestPatchCoordinatorRequestUnknownKeysDoNotAffectKnownFields(t *testing.T) {
	// Build decision 7: "unknown ignored". A caller only ever looks up the
	// four known field names, so an extra body key (here, one that isn't
	// even a coordinator field) must leave a known field's own presence
	// check unaffected.
	var req PatchCoordinatorRequest
	if err := json.Unmarshal([]byte(`{"unknown_field": "value"}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	_, present, err := req.StringField(PatchFieldName)
	if err != nil {
		t.Fatalf("StringField() unexpected error: %v", err)
	}
	if present {
		t.Errorf("present = true, want false: %q was never sent", PatchFieldName)
	}
}

func TestProposalDTONeverSerializesClaimToken(t *testing.T) {
	token := "secret-token"
	p := &Proposal{
		ID: "prop-1", CoordinatorID: "co-1", WorkspaceID: "ws-1", Status: ProposalStatusApproving,
		Spec: ProposalSpec{Title: "Do the thing"}, ClaimToken: &token,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	raw, err := json.Marshal(NewProposalDTO(p))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if _, ok := decoded["claim_token"]; ok {
		t.Errorf("marshaled ProposalDTO unexpectedly has key %q: %s", "claim_token", raw)
	}
	wantKeys := []string{"id", "coordinator_id", "workspace_id", "status", "spec", "final_spec", "claimed_at", "task_id", "error", "reject_reason", "decided_by", "created_at", "updated_at"}
	for _, key := range wantKeys {
		if _, ok := decoded[key]; !ok {
			t.Errorf("marshaled ProposalDTO missing key %q: %s", key, raw)
		}
	}
}

func TestNewProposalListResponseNeverSerializesNull(t *testing.T) {
	raw, err := json.Marshal(NewProposalListResponse(nil))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if string(raw) != `{"proposals":[]}` {
		t.Errorf("Marshal(empty list) = %s, want {\"proposals\":[]}", raw)
	}
}

func TestNewStallListResponseNeverSerializesNull(t *testing.T) {
	raw, err := json.Marshal(NewStallListResponse(nil))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if string(raw) != `{"stalls":[]}` {
		t.Errorf("Marshal(empty list) = %s, want {\"stalls\":[]}", raw)
	}
}

func TestStallDTOFields(t *testing.T) {
	lastEvent := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	detected := lastEvent.Add(time.Minute)
	dto := NewStallDTO(&Stall{TaskID: "task-1", WorkspaceID: "ws-1", StalledForMs: 120000, LastEventAt: lastEvent, DetectedAt: detected})
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if _, ok := decoded["workspace_id"]; ok {
		t.Errorf("marshaled StallDTO unexpectedly has key %q (needs-you.md's route body omits it): %s", "workspace_id", raw)
	}
	for _, key := range []string{"task_id", "stalled_for_ms", "last_event_at", "detected_at"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("marshaled StallDTO missing key %q: %s", key, raw)
		}
	}
}

// pinProposalConflictResponseKeys is decision 16's "JSON round-trip test
// pinning the three keys": exactly error, error_code and proposal, no more,
// no fewer.
func TestProposalConflictResponsePinsThreeKeys(t *testing.T) {
	p := &Proposal{ID: "prop-1", CoordinatorID: "co-1", WorkspaceID: "ws-1", Status: ProposalStatusPending, Spec: ProposalSpec{Title: "x"}}
	raw, err := json.Marshal(NewProposalConflictResponse(p))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if len(decoded) != 3 {
		t.Fatalf("len(decoded) = %d, want 3: %s", len(decoded), raw)
	}
	if decoded["error"] != "proposal_conflict" {
		t.Errorf("error = %v, want %q", decoded["error"], "proposal_conflict")
	}
	if decoded["error_code"] != "proposal_conflict" {
		t.Errorf("error_code = %v, want %q", decoded["error_code"], "proposal_conflict")
	}
	proposal, ok := decoded["proposal"].(map[string]any)
	if !ok {
		t.Fatalf("proposal field is not an object: %s", raw)
	}
	if proposal["id"] != "prop-1" {
		t.Errorf("proposal.id = %v, want %q", proposal["id"], "prop-1")
	}
}

func TestConversationConflictResponsePinsTwoKeys(t *testing.T) {
	raw, err := json.Marshal(NewConversationConflictResponse())
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if string(raw) != `{"error":"conversation_conflict","error_code":"conversation_conflict"}` {
		t.Errorf("Marshal() = %s, want the two-key conversation_conflict body", raw)
	}
}

func TestCoordinatorProfileUnavailableResponse(t *testing.T) {
	raw, err := json.Marshal(NewCoordinatorProfileUnavailableResponse(ProfileStatusMissing, ProfileStatusOK))
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if decoded["error"] != "coordinator_profile_unavailable" {
		t.Errorf("error = %v, want %q", decoded["error"], "coordinator_profile_unavailable")
	}
	if decoded["agent_profile_status"] != "missing" {
		t.Errorf("agent_profile_status = %v, want %q", decoded["agent_profile_status"], "missing")
	}
	if decoded["executor_profile_status"] != "ok" {
		t.Errorf("executor_profile_status = %v, want %q", decoded["executor_profile_status"], "ok")
	}
}

func TestConversationResponseFields(t *testing.T) {
	raw, err := json.Marshal(&ConversationResponse{TaskID: "task-1", SessionID: "session-1", ArchiveState: false})
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if string(raw) != `{"task_id":"task-1","session_id":"session-1","archive_state":false}` {
		t.Errorf("Marshal() = %s, want the pinned conversation response body", raw)
	}
}

func TestApproveProposalRequestStringFieldAbsent(t *testing.T) {
	req := ApproveProposalRequest{}
	value, present, err := req.StringField(ApproveFieldTitle)
	if err != nil {
		t.Fatalf("StringField() unexpected error: %v", err)
	}
	if present {
		t.Errorf("present = true, want false for an absent field")
	}
	if value != nil {
		t.Errorf("value = %v, want nil for an absent field", value)
	}
}

func TestApproveProposalRequestStringFieldNull(t *testing.T) {
	var req ApproveProposalRequest
	if err := json.Unmarshal([]byte(`{"title": null}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	value, present, err := req.StringField(ApproveFieldTitle)
	if !present {
		t.Errorf("present = false, want true for a field sent as null")
	}
	if value != nil {
		t.Errorf("value = %v, want nil for a field sent as null", value)
	}
	var fieldErr *FieldError
	if err == nil {
		t.Fatal("StringField() error = nil, want a *FieldError for JSON null")
	}
	if !errors.As(err, &fieldErr) {
		t.Fatalf("StringField() error type = %T, want *FieldError", err)
	}
	if fieldErr.Field != ApproveFieldTitle {
		t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, ApproveFieldTitle)
	}
}

func TestApproveProposalRequestStringFieldValue(t *testing.T) {
	var req ApproveProposalRequest
	if err := json.Unmarshal([]byte(`{"repository_id": ""}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	value, present, err := req.StringField(ApproveFieldRepositoryID)
	if err != nil {
		t.Fatalf("StringField() unexpected error: %v", err)
	}
	if !present {
		t.Errorf("present = false, want true for a sent field")
	}
	if value == nil || *value != "" {
		t.Errorf("value = %v, want a pointer to the empty string", value)
	}
}

func TestApproveProposalRequestStringFieldWrongType(t *testing.T) {
	var req ApproveProposalRequest
	if err := json.Unmarshal([]byte(`{"step_id": 42}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	_, _, err := req.StringField(ApproveFieldStepID)
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("StringField() error type = %T, want *FieldError", err)
	}
	if fieldErr.Field != ApproveFieldStepID {
		t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, ApproveFieldStepID)
	}
}

func TestApproveProposalRequestUnknownKeysDoNotAffectKnownFields(t *testing.T) {
	var req ApproveProposalRequest
	if err := json.Unmarshal([]byte(`{"rationale": "ignored", "source_task_id": "ignored"}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	_, present, err := req.StringField(ApproveFieldWorkflowID)
	if err != nil {
		t.Fatalf("StringField() unexpected error: %v", err)
	}
	if present {
		t.Errorf("present = true, want false: %q was never sent", ApproveFieldWorkflowID)
	}
}

func TestRejectProposalRequestReasonAbsent(t *testing.T) {
	var req RejectProposalRequest
	if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if req.Reason != nil {
		t.Errorf("Reason = %v, want nil for an absent reason", req.Reason)
	}
}

func TestRejectProposalRequestReasonNull(t *testing.T) {
	var req RejectProposalRequest
	if err := json.Unmarshal([]byte(`{"reason": null}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if req.Reason != nil {
		t.Errorf("Reason = %v, want nil for a null reason", req.Reason)
	}
}

func TestRejectProposalRequestReasonValue(t *testing.T) {
	var req RejectProposalRequest
	if err := json.Unmarshal([]byte(`{"reason": "too busy"}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if req.Reason == nil || *req.Reason != "too busy" {
		t.Errorf("Reason = %v, want a pointer to %q", req.Reason, "too busy")
	}
}
