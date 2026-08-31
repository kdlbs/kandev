package plugins

import (
	"testing"
	"time"
)

func TestApprovalDTOConversionCopiesState(t *testing.T) {
	createdAt := time.Date(2026, time.August, 31, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	dto := approvalDTOFromCurrent(CapabilityApproval{
		InstallationID:     "inst-1",
		WorkspaceID:        "ws-1",
		Revision:           7,
		ManifestDigest:     "digest",
		CapabilityIDs:      []string{"api_read:tasks"},
		State:              ApprovalStateActive,
		HumanActor:         "human",
		HumanPolicyVersion: "immutable",
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	})
	if dto.InstallationID != "inst-1" || dto.WorkspaceID != "ws-1" || dto.Revision != 7 {
		t.Fatalf("dto = %#v", dto)
	}
}

func TestApprovalEventDTOConversionCopiesAuditIdentity(t *testing.T) {
	when := time.Date(2026, time.August, 31, 1, 2, 3, 0, time.UTC)
	dto := approvalEventDTOFromCurrent(CapabilityApprovalEvent{
		AuditID:        "audit-1",
		InstallationID: "inst-1",
		WorkspaceID:    "ws-1",
		BeforeRevision: 1,
		AfterRevision:  2,
		BeforeDigest:   "a",
		AfterDigest:    "b",
		Actor:          "human",
		Reason:         "grant",
		Type:           CapabilityApprovalEventGrant,
		ObservedAt:     when,
	})
	if dto.AuditID != "audit-1" || dto.Type != string(CapabilityApprovalEventGrant) {
		t.Fatalf("dto = %#v", dto)
	}
}
