package winproc

import (
	"os"
	"syscall"
	"testing"
)

func TestShutdownPairingPrefersWaitingOSSignal(t *testing.T) {
	var pairing ShutdownPairing
	osSignals := make(chan os.Signal, 1)
	osSignals <- syscall.SIGTERM

	if got := pairing.CooperativeFirst(os.Interrupt, osSignals); got != syscall.SIGTERM {
		t.Fatalf("first signal = %v, want waiting OS signal %v", got, syscall.SIGTERM)
	}
	if !pairing.Forces(os.Interrupt) {
		t.Fatal("next OS signal after an OS first signal must force exit")
	}
}

func TestShutdownPairingAbsorbsOneOSSignalAfterCooperativeFirst(t *testing.T) {
	var pairing ShutdownPairing
	if got := pairing.CooperativeFirst(os.Interrupt, make(chan os.Signal)); got != os.Interrupt {
		t.Fatalf("first signal = %v, want cooperative %v", got, os.Interrupt)
	}
	if pairing.Forces(os.Interrupt) {
		t.Fatal("OS signal paired with the cooperative request forced exit")
	}
	if !pairing.Forces(os.Interrupt) {
		t.Fatal("deliberate second OS signal did not force exit")
	}
}

func TestShutdownPairingZeroValueForcesOnSecondOSSignal(t *testing.T) {
	var pairing ShutdownPairing
	if !pairing.Forces(os.Interrupt) {
		t.Fatal("second OS signal after an OS first signal must force exit")
	}
}
