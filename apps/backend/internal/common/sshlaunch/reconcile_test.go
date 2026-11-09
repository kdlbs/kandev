package sshlaunch

import "testing"

func TestReconcileResultValidate(t *testing.T) {
	valid := []ReconcileResult{
		{Outcome: OutcomeStopped},
		{Outcome: OutcomeAbsent},
		{Outcome: OutcomeVerified},
		Unresolved(ReasonIdentityConflict, ""),
		Unresolved(ReasonLaunchIdentityMissing, ""),
		Unresolved(ReasonUnsupportedIdentity, ""),
		Unresolved(ReasonProbeUnknown, ""),
		Unresolved(ReasonTerminationUnconfirmed, ""),
	}
	for _, result := range valid {
		if err := result.Validate(); err != nil {
			t.Fatalf("Validate(%#v) = %v", result, err)
		}
	}
	invalid := []ReconcileResult{
		{Outcome: "success"},
		{Outcome: OutcomeStopped, Reason: ReasonProbeUnknown},
		{Outcome: OutcomeUnresolved},
		{Outcome: OutcomeUnresolved, Reason: "made_up"},
	}
	for _, result := range invalid {
		if err := result.Validate(); err == nil {
			t.Fatalf("Validate(%#v) succeeded, want an error", result)
		}
	}
}

func TestReconcileResultTerminal(t *testing.T) {
	if !(ReconcileResult{Outcome: OutcomeStopped}).Terminal() {
		t.Fatal("stopped must be terminal")
	}
	if !(ReconcileResult{Outcome: OutcomeAbsent}).Terminal() {
		t.Fatal("absent must be terminal")
	}
	if Unresolved(ReasonProbeUnknown, "").Terminal() {
		t.Fatal("unresolved must not be terminal")
	}
	if (ReconcileResult{Outcome: OutcomeVerified}).Terminal() {
		t.Fatal("a verify result must not settle cleanup")
	}
}

func TestReconcileRequestValidate(t *testing.T) {
	receipt := testReceipt(t)
	if err := (ReconcileRequest{Version: Version, Receipt: receipt}).Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := (ReconcileRequest{Version: Version, Mode: ReconcileModeVerify, Receipt: receipt}).Validate(); err != nil {
		t.Fatalf("valid verify request rejected: %v", err)
	}
	if err := (ReconcileRequest{Version: Version, Mode: "delete", Receipt: receipt}).Validate(); err == nil {
		t.Fatal("unknown mode accepted")
	}
	if (ReconcileMode("")).Effective() != ReconcileModeTerminate {
		t.Fatal("empty mode must default to terminate")
	}
	if err := (ReconcileRequest{Version: Version + 1, Receipt: receipt}).Validate(); err == nil {
		t.Fatal("unsupported request version accepted")
	}
	broken := receipt
	broken.Process.PID = 0
	if err := (ReconcileRequest{Version: Version, Receipt: broken}).Validate(); err == nil {
		t.Fatal("incomplete receipt accepted")
	}
}
