package streams

import (
	"encoding/json"
	"testing"
)

func TestBackgroundWorkAttestationRoundTrip(t *testing.T) {
	payload := NewSubagentTask("audit", "inspect lifecycle", "reviewer")
	payload.SetBackgroundWorkIdentity(BackgroundWorkKindSubagent, "child-1", false, false)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded NormalizedPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	background := decoded.BackgroundWork()
	if background == nil {
		t.Fatal("background-work attestation was lost")
	}
	if background.Kind != BackgroundWorkKindSubagent || background.WorkID != "child-1" {
		t.Fatalf("background-work attestation = %+v", background)
	}
	if !decoded.IsActiveBackgroundWork() {
		t.Fatal("live attestation became inactive after round trip")
	}
}

func TestBackgroundWorkAttestationDistinguishesDetachedAndEnded(t *testing.T) {
	payload := NewShellExec("sleep 30", "", "", 0, true)
	payload.SetBackgroundWorkIdentity(BackgroundWorkKindShell, "job-1", true, false)
	if !payload.IsActiveBackgroundWork() || !payload.IsDetachedBackgroundLaunch() {
		t.Fatal("live detached shell was not classified as detached background work")
	}

	payload.SetBackgroundWorkIdentity(BackgroundWorkKindShell, "", true, true)
	if payload.IsActiveBackgroundWork() || payload.IsDetachedBackgroundLaunch() {
		t.Fatal("ended shell remained active")
	}
	if got := payload.BackgroundWork().WorkID; got != "job-1" {
		t.Fatalf("empty update erased work ID: %q", got)
	}
}

// TestBackgroundWorkLegacyWireWithoutUnverifiedStaysVerified pins AC-001.3:
// an older agentctl (or a retained journal frame) that serialized a recognized
// workload without the negative marker keeps the legacy verified meaning, so
// the parked projection is not suppressed.
func TestBackgroundWorkLegacyWireWithoutUnverifiedStaysVerified(t *testing.T) {
	var payload NormalizedPayload
	if err := json.Unmarshal([]byte(`{
		"kind": "shell_exec",
		"shell_exec": {"command": "sleep 30", "background": true},
		"background_work": {"kind": "shell", "work_id": "shell-1", "detached": true}
	}`), &payload); err != nil {
		t.Fatalf("unmarshal legacy wire shape: %v", err)
	}
	if payload.BackgroundWork() == nil {
		t.Fatal("legacy attestation was lost")
	}
	if !payload.IsVerifiedBackgroundWork() {
		t.Fatal("legacy wire shape without the unverified marker must stay verified")
	}
}

// TestBackgroundWorkExplicitUnverifiedMarker pins that the new provider-owned
// negative marker survives serialization and cannot attest live activity.
func TestBackgroundWorkExplicitUnverifiedMarker(t *testing.T) {
	payload := NewShellExec("sleep 30", "", "", 0, true)
	payload.SetUnverifiedBackgroundWorkIdentity(BackgroundWorkKindShell, "shell-1", true, false)
	payload.SetBackgroundWorkRunID("shell-1")
	payload.SetBackgroundWorkCapabilities(WorkloadCapabilities{
		Discovery: "events_only",
		Output:    "none",
	})

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded NormalizedPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.IsVerifiedBackgroundWork() {
		t.Fatal("explicitly unverified work must not report verified")
	}
	background := decoded.BackgroundWork()
	if background == nil || background.RunID != "shell-1" {
		t.Fatalf("provider run identity lost: %+v", background)
	}
	if background.Capabilities == nil || background.Capabilities.Output != "none" {
		t.Fatalf("capability declaration lost: %+v", background)
	}
}

// TestBackgroundWorkRunIDIsNeverDerived pins that the payload carries only the
// provider's own run identity; callers must not infer one from WorkID.
func TestBackgroundWorkRunIDIsNeverDerived(t *testing.T) {
	payload := NewShellExec("sleep 30", "", "", 0, true)
	payload.SetBackgroundWorkIdentity(BackgroundWorkKindShell, "item-1", true, false)
	if got := payload.BackgroundWork().RunID; got != "" {
		t.Fatalf("RunID = %q, want empty without an explicit provider run key", got)
	}
	payload.SetBackgroundWorkRunID("process-1")
	if got := payload.BackgroundWork().RunID; got != "process-1" {
		t.Fatalf("RunID = %q, want process-1", got)
	}
}

// TestMonitorIdentityDeclaresInspectionOnlySurface pins that a Monitor producer
// publishes its explicit inspect-only contract with the identity.
func TestMonitorIdentityDeclaresInspectionOnlySurface(t *testing.T) {
	payload := NewGeneric("Monitor", map[string]any{})
	payload.SetMonitorIdentity("monitor-1", false)
	work := payload.BackgroundWork()
	if work == nil || work.Capabilities == nil {
		t.Fatal("monitor identity must declare its capability surface")
	}
	if work.Capabilities.Output != "none" || len(work.Capabilities.Actions) != 0 {
		t.Fatalf("monitor capabilities = %+v, want inspect-only", work.Capabilities)
	}
}
