package client

import (
	"testing"
	"time"
)

type recoveryStartedBinding struct {
	candidate *RuntimeBindingCandidate
	notify    RuntimeLossHandler
	number    int
}

func waitRecoveryBinding(t *testing.T, started <-chan recoveryStartedBinding) recoveryStartedBinding {
	t.Helper()
	select {
	case binding := <-started:
		return binding
	case <-time.After(2 * time.Second):
		t.Fatal("runtime recovery binding did not start")
		return recoveryStartedBinding{}
	}
}
