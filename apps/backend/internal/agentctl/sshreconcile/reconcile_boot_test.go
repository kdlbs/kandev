//go:build linux

package sshreconcile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.9
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestReconcileVerifyPreviousBootController(t *testing.T) {
	t.Run("absent pid", func(t *testing.T) {
		taskDir := t.TempDir()
		generation := newGeneration(t)
		controller := startOwnedController(t, generation, taskDir)
		receipt := previousBootReceipt(t, ownedReceipt(t, controller, generation, taskDir, filepath.Join(taskDir, "sessions", "session-1")))
		if err := controller.command.Process.Kill(); err != nil {
			t.Fatalf("stop owned controller: %v", err)
		}
		if _, err := controller.command.Process.Wait(); err != nil {
			t.Fatalf("wait for owned controller: %v", err)
		}

		result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
			Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeVerify, Receipt: receipt,
		})
		if result.Outcome != sshlaunch.OutcomeAbsent {
			t.Fatalf("result = %#v, want absent", result)
		}
	})

	t.Run("reused pid and replacement directory", func(t *testing.T) {
		taskDir := t.TempDir()
		sessionDir := filepath.Join(taskDir, "sessions", "session-1")
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("create replacement session directory: %v", err)
		}
		marker := filepath.Join(sessionDir, "replacement.marker")
		if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
			t.Fatalf("write replacement marker: %v", err)
		}
		generation := newGeneration(t)
		controller := startOwnedController(t, generation, taskDir)
		receipt := previousBootReceipt(t, ownedReceipt(t, controller, generation, taskDir, sessionDir))

		result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
			Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeVerify, Receipt: receipt,
		})
		if result.Outcome != sshlaunch.OutcomeAbsent {
			t.Fatalf("result = %#v, want absent", result)
		}
		if !controller.alive() {
			t.Fatal("verification signalled the current holder of the reused PID")
		}
		if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve" {
			t.Fatalf("replacement directory marker = %q, %v; want preserve", data, err)
		}
	})
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.10
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.23
func TestReconcileVerifyInvalidBootEvidence(t *testing.T) {
	taskDir := t.TempDir()
	generation := newGeneration(t)
	controller := startOwnedController(t, generation, taskDir)
	receipt := ownedReceipt(t, controller, generation, taskDir, filepath.Join(taskDir, "sessions", "session-1"))
	receipt.Process.BirthToken = "invalid"

	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeVerify, Receipt: receipt,
	})
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonUnsupportedIdentity {
		t.Fatalf("result = %#v, want unresolved unsupported_identity", result)
	}
	if !controller.alive() {
		t.Fatal("invalid boot evidence signalled the current process")
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestReconcileTerminatePreviousBootRefuses(t *testing.T) {
	taskDir := t.TempDir()
	generation := newGeneration(t)
	controller := startOwnedController(t, generation, taskDir)
	receipt := previousBootReceipt(t, ownedReceipt(t, controller, generation, taskDir, filepath.Join(taskDir, "sessions", "session-1")))

	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeTerminate, Receipt: receipt,
	})
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonIdentityConflict {
		t.Fatalf("result = %#v, want unresolved identity_conflict", result)
	}
	if !controller.alive() {
		t.Fatal("terminate mode signalled a current PID holder for a previous-boot receipt")
	}
}

func previousBootReceipt(t *testing.T, receipt sshlaunch.Receipt) sshlaunch.Receipt {
	t.Helper()
	parts := strings.Split(receipt.Process.BirthToken, ":")
	if len(parts) != 3 {
		t.Fatalf("unexpected birth token %q", receipt.Process.BirthToken)
	}
	parts[1] = "previous-" + parts[1]
	receipt.Process.BirthToken = strings.Join(parts, ":")
	return receipt
}
