package service

import (
	"errors"
	"fmt"
	"testing"
)

type recoverySignalForTest struct{}

func (recoverySignalForTest) Error() string { return "native state is unavailable" }

func (recoverySignalForTest) RecoveryReason() string { return "native_state_missing" }

func TestSessionRecoveryDetailsClassifiesWrappedLaunchFailure(t *testing.T) {
	reason, ok := sessionRecoveryDetails(fmt.Errorf("launch: %w", recoverySignalForTest{}))
	if !ok {
		t.Fatal("session recovery error was not classified")
	}
	if reason != "native_state_missing" {
		t.Fatalf("reason = %q, want native_state_missing", reason)
	}
}

func TestSessionRecoveryDetailsIgnoresOrdinaryLaunchFailure(t *testing.T) {
	if reason, ok := sessionRecoveryDetails(errors.New("provider unavailable")); ok || reason != "" {
		t.Fatalf("ordinary error classified as recovery: reason=%q ok=%t", reason, ok)
	}
}
