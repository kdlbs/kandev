package plugins

import (
	"strings"
	"testing"
	"time"
)

func TestCanonicalCapabilityListSortsDeduplicatesAndTrims(t *testing.T) {
	got, err := CanonicalCapabilityList([]string{" host.v2.write:tasks ", "host.v2.read:tasks", "host.v2.write:tasks"})
	if err != nil {
		t.Fatalf("CanonicalCapabilityList() unexpected error: %v", err)
	}
	want := []string{"host.v2.read:tasks", "host.v2.write:tasks"}
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
	if _, err := CanonicalCapabilityList([]string{"host.v2.read:*"}); err == nil {
		t.Fatal("CanonicalCapabilityList() accepted a wildcard capability")
	}
}

func TestCanonicalCapabilityListRequiresExactHostV2Capabilities(t *testing.T) {
	for _, capability := range []string{
		"api_read:tasks",
		"api_write:tasks",
		"host.v2.read",
		"host.v2.read:tasks:extra",
		"host.v2.admin:tasks",
	} {
		if _, err := CanonicalCapabilityList([]string{capability}); err == nil {
			t.Fatalf("CanonicalCapabilityList(%q) accepted a non-exact H6 capability", capability)
		}
	}

	got, err := CanonicalCapabilityList([]string{"host.v2.write:tasks", "host.v2.read:tasks"})
	if err != nil {
		t.Fatalf("CanonicalCapabilityList() rejected exact H6 capabilities: %v", err)
	}
	want := []string{"host.v2.read:tasks", "host.v2.write:tasks"}
	if !equalStrings(got, want) {
		t.Fatalf("capabilities = %#v, want %#v", got, want)
	}
}

func TestCanonicalCapabilityListRejectsNamespacedHumanReservedCapabilities(t *testing.T) {
	for _, capability := range []string{
		"api_write:merge",
		"host.v2.write:merge",
		"host.v2.write:deploy",
		"host.v2.write:release",
		"host.v2.write:rewrite_history",
		"host.v2.write:cross_workspace",
		"host.v2.write:secret_scope_expand",
	} {
		if _, err := CanonicalCapabilityList([]string{capability}); err == nil {
			t.Fatalf("CanonicalCapabilityList(%q) accepted a Human-reserved capability", capability)
		}
	}
}

func TestAuthorizePluginCapabilityDoesNotTreatV1DeclarationsAsH6Authority(t *testing.T) {
	svc := &Service{}
	svc.SetPluginsDir(t.TempDir())
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"host.v2.write:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	for _, capabilityID := range []string{"api_read:tasks", "api_write:tasks"} {
		decision := svc.authorizePluginCapability("inst-1", "ws-1", capabilityID, 1, "request", "method")
		if decision.Allowed || decision.Reason != ApprovalDenyUnsupportedCapability {
			t.Fatalf("legacy %q decision = %#v, want unsupported denial", capabilityID, decision)
		}
	}
	for _, capabilityID := range []string{"api_write:merge", "host.v2.write:merge"} {
		decision := svc.authorizePluginCapability("inst-1", "ws-1", capabilityID, 1, "request", "method")
		if decision.Allowed || decision.Reason != ApprovalDenyHumanReserved {
			t.Fatalf("reserved %q decision = %#v, want Human-policy denial", capabilityID, decision)
		}
	}
}

func TestCanonicalCapabilityListBoundsCapabilityInputs(t *testing.T) {
	tooLong := "host.v2.read:" + strings.Repeat("a", 1024)
	if _, err := CanonicalCapabilityList([]string{tooLong}); err == nil {
		t.Fatal("CanonicalCapabilityList() accepted an oversized capability")
	}

	capabilities := make([]string, 1025)
	for i := range capabilities {
		capabilities[i] = "host.v2.read:tasks"
	}
	if _, err := CanonicalCapabilityList(capabilities); err == nil {
		t.Fatal("CanonicalCapabilityList() accepted too many capability inputs")
	}
}

func TestApprovalLedgerRejectsOversizedOrMalformedAuditMetadata(t *testing.T) {
	ledger := newApprovalLedger(t.TempDir())
	for _, tc := range []struct {
		name   string
		actor  string
		reason string
		audit  string
	}{
		{name: "oversized actor", actor: strings.Repeat("a", 1025), reason: "grant", audit: "audit-1"},
		{name: "oversized reason", actor: "human", reason: strings.Repeat("a", 1025), audit: "audit-1"},
		{name: "nul audit id", actor: "human", reason: "grant", audit: "audit\x00secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"host.v2.read:tasks"}, tc.actor, tc.reason, tc.audit, time.Now().UTC()); err == nil {
				t.Fatal("grant accepted unsafe audit metadata")
			}
		})
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
		CapabilityID:   "host.v2.read:tasks",
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

func TestAuthorizePluginCapabilityCopiesAuditIDToDeniedDecision(t *testing.T) {
	svc := &Service{}
	svc.SetPluginsDir(t.TempDir())
	decision := svc.authorizePluginCapability("inst-1", "ws-1", "host.v2.read:tasks", 1, "req", "method")
	if decision.AuditID == "" || decision.AuditID != decision.Receipt.AuditID {
		t.Fatalf("denied decision audit id = %q, receipt audit id = %q", decision.AuditID, decision.Receipt.AuditID)
	}
}

func TestAuthorizePluginCapabilityBoundsAndDigestsReceiptInputs(t *testing.T) {
	svc := &Service{}
	svc.SetPluginsDir(t.TempDir())
	secret := "token=super-secret-value"
	decision := svc.authorizePluginCapability("inst-1", "ws-1", "host.v2.read:tasks", 1, secret, "method=secret")
	if decision.Receipt.RequestDigest == secret || decision.Receipt.MethodDigest == "method=secret" {
		t.Fatalf("receipt leaked request input: %#v", decision.Receipt)
	}
	if len(decision.Receipt.RequestDigest) != 64 || len(decision.Receipt.MethodDigest) != 64 {
		t.Fatalf("receipt digest lengths = %d, %d; want SHA-256 hex", len(decision.Receipt.RequestDigest), len(decision.Receipt.MethodDigest))
	}

	tooLong := strings.Repeat("a", 1025)
	for _, tc := range []struct {
		name string
		call func() ApprovalDecision
	}{
		{"installation", func() ApprovalDecision {
			return svc.authorizePluginCapability(tooLong, "ws-1", "host.v2.read:tasks", 1, "request", "method")
		}},
		{"workspace", func() ApprovalDecision {
			return svc.authorizePluginCapability("inst-1", tooLong, "host.v2.read:tasks", 1, "request", "method")
		}},
		{"request", func() ApprovalDecision {
			return svc.authorizePluginCapability("inst-1", "ws-1", "host.v2.read:tasks", 1, strings.Repeat("a", 4097), "method")
		}},
		{"method", func() ApprovalDecision {
			return svc.authorizePluginCapability("inst-1", "ws-1", "host.v2.read:tasks", 1, "request", strings.Repeat("a", 4097))
		}},
		{"nul request", func() ApprovalDecision {
			return svc.authorizePluginCapability("inst-1", "ws-1", "host.v2.read:tasks", 1, "request\x00secret", "method")
		}},
		{"nul method", func() ApprovalDecision {
			return svc.authorizePluginCapability("inst-1", "ws-1", "host.v2.read:tasks", 1, "request", "method\x00secret")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if decision := tc.call(); decision.Reason != ApprovalDenyMalformedRequest {
				t.Fatalf("reason = %q, want malformed_request", decision.Reason)
			}
		})
	}
	malformed := svc.authorizePluginCapability(tooLong, "ws-1", "host.v2.read:tasks", 1, "request", "method")
	if malformed.Receipt.InstallationID != "" {
		t.Fatalf("receipt leaked malformed installation input: %#v", malformed.Receipt)
	}
}
