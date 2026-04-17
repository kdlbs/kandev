package hostutility

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestClassifyProbeError_Timeout(t *testing.T) {
	status, msg := classifyProbeError(errors.New("probe: context deadline exceeded"), context.DeadlineExceeded)
	if status != StatusFailed {
		t.Errorf("expected StatusFailed for timeout, got %s", status)
	}
	if !strings.Contains(msg, "timed out") {
		t.Errorf("expected timeout message, got %q", msg)
	}
}

func TestClassifyProbeError_ExecutableNotFound(t *testing.T) {
	// Mirrors the string format from acp_executor's cmd.Start() wrap:
	// `start: exec: "<name>": executable file not found in $PATH`.
	err := errors.New(`start: exec: "mock-agent": executable file not found in $PATH`)
	status, msg := classifyProbeError(err, nil)
	if status != StatusNotInstalled {
		t.Errorf("expected StatusNotInstalled for missing binary, got %s", status)
	}
	if msg != err.Error() {
		t.Errorf("expected original message preserved, got %q", msg)
	}
}

func TestClassifyProbeError_Generic(t *testing.T) {
	err := errors.New("rpc: connection refused")
	status, msg := classifyProbeError(err, nil)
	if status != StatusFailed {
		t.Errorf("expected StatusFailed for generic error, got %s", status)
	}
	if msg != err.Error() {
		t.Errorf("expected original message preserved, got %q", msg)
	}
}
