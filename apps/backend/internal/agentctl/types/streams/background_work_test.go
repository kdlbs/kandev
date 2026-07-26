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
