package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.21
func TestSSHReconcileCommandRejectsMalformedInput(t *testing.T) {
	var stdout bytes.Buffer
	if code := runSSHReconcile(context.Background(), strings.NewReader("{not-json"), &stdout); code != 0 {
		t.Fatalf("exit code = %d, want 0 with an unresolved result", code)
	}
	var result sshlaunch.ReconcileResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v (body %q)", err, stdout.String())
	}
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonProbeUnknown {
		t.Fatalf("result = %#v, want unresolved probe_unknown", result)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.21
func TestSSHReconcileCommandRejectsOversizedInput(t *testing.T) {
	var stdout bytes.Buffer
	oversized := strings.Repeat("x", maxSSHReconcileRequestBytes+1)
	if code := runSSHReconcile(context.Background(), strings.NewReader(oversized), &stdout); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), string(sshlaunch.ReasonProbeUnknown)) {
		t.Fatalf("result = %q, want probe_unknown", stdout.String())
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.22
func TestSSHReconcileCommandCompletesForAnAbsentProcess(t *testing.T) {
	generation, err := sshlaunch.NewGeneration()
	if err != nil {
		t.Fatalf("NewGeneration: %v", err)
	}
	request := sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version,
		Receipt: sshlaunch.Receipt{
			Version:    sshlaunch.Version,
			Generation: generation,
			Owner: sshlaunch.Owner{
				TaskID:               "task-1",
				SessionID:            "session-1",
				ControllerInstanceID: "instance-1",
				TaskDir:              "/nonexistent/task",
				SessionDir:           "/nonexistent/task/session",
				Target: sshlaunch.TargetBinding{
					Host:            "runner.example.test",
					Port:            "22",
					User:            "kandev",
					HostFingerprint: "SHA256:test",
				},
			},
			UID: os.Geteuid(),
			Process: processidentity.Identity{
				PID:        1 << 30,
				GroupID:    1 << 30,
				SessionID:  1 << 30,
				BirthToken: "linux:test-boot:1",
			},
		},
	}
	body, _ := json.Marshal(request)
	var stdout bytes.Buffer
	if code := runSSHReconcile(context.Background(), bytes.NewReader(body), &stdout); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	var result sshlaunch.ReconcileResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	// The recorded PID is either absent (absent) or the probe fails closed
	// (unresolved). Both are terminal-safe for the caller; neither may be
	// stopped.
	if result.Outcome == sshlaunch.OutcomeStopped {
		t.Fatalf("result = %#v, an unproven process may never be reported stopped", result)
	}
}
