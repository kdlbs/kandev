package plugins

import (
	"testing"
	"time"
)

func TestCanonicalCapabilityListSortsDeduplicatesAndTrims(t *testing.T) {
	got, err := CanonicalCapabilityList([]string{" api_write:tasks ", "api_read:tasks", "api_write:tasks"})
	if err != nil {
		t.Fatalf("CanonicalCapabilityList() unexpected error: %v", err)
	}
	want := []string{"api_read:tasks", "api_write:tasks"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCanonicalCapabilityListRejectsWildcards(t *testing.T) {
	if _, err := CanonicalCapabilityList([]string{"api_read:*"}); err == nil {
		t.Fatal("CanonicalCapabilityList() accepted a wildcard capability")
	}
}

func TestCanonicalApprovalDigestIsStable(t *testing.T) {
	got1 := CanonicalApprovalDigest(" installation ", "workspace", "7")
	got2 := CanonicalApprovalDigest("installation", " workspace ", "7")
	if got1 != got2 {
		t.Fatalf("CanonicalApprovalDigest() is not stable: %q != %q", got1, got2)
	}
}

func TestApprovalReceiptCarriesSafeMetadata(t *testing.T) {
	when := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	receipt := ApprovalReceipt{
		InstallationID: "inst-1",
		WorkspaceID:    "ws-1",
		Revision:       3,
		CapabilityID:   "api_read:tasks",
		RequestDigest:  "req",
		MethodDigest:   "method",
		AuditID:        "audit-1",
		Result:         "allowed",
		ObservedAt:     when,
	}
	if receipt.InstallationID != "inst-1" || receipt.WorkspaceID != "ws-1" || receipt.Revision != 3 {
		t.Fatalf("receipt = %#v, want safe identity fields preserved", receipt)
	}
	if receipt.ObservedAt != when {
		t.Fatalf("ObservedAt = %v, want %v", receipt.ObservedAt, when)
	}
}
