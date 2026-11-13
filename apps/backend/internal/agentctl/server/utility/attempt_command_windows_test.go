//go:build windows

package utility

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"go.uber.org/zap"
)

func TestAttemptActualWindowsChildNeedsJobTerminationProof(t *testing.T) {
	owner := newAttemptOwner(&config.InstanceConfig{InstanceID: "instance", InferenceOwnerFresh: true}, nil)
	prepared, err := owner.prepare(uuid.NewString(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.consume(prepared.AttemptID); err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := withAttempt(parent, owner, prepared.AttemptID)
	cmd := exec.CommandContext(context.Background(), os.Args[0])
	cmd.Env = append(os.Environ(), acpCommandTestFixtureEnv+"=cat")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	configureACPCommand(cmd, zap.NewNop())
	if err := startInferenceCommand(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := installACPCommandLifecycle(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			cleanupACPCommand(ctx, cmd, lifecycle, zap.NewNop())
		}
	})
	markInferenceDispatch(ctx)
	cancel()
	observed, ok := owner.get(prepared.AttemptID)
	if !ok || observed.Settled || observed.State != AttemptRunning || observed.PromptDispatch != PromptPossiblySent {
		t.Fatalf("parent cancellation invented proof: %+v", observed)
	}
	if _, err := owner.cancelPrepared(prepared.AttemptID, ""); err == nil {
		t.Fatal("cancelPrepared released live child")
	}
	cleanupACPCommand(ctx, cmd, lifecycle, zap.NewNop())
	cleaned = true
	observed, ok = owner.get(prepared.AttemptID)
	if !ok || !observed.Known || !observed.Settled || observed.State != AttemptTerminated {
		t.Fatalf("actual Job completion did not retain exact terminal proof: %+v", observed)
	}
	if recoveredInferenceTerminated(cmd.Process.Pid) {
		t.Fatal("lost Windows Job handle was accepted as recovered tree proof")
	}
}

func TestAttemptActualWindowsStartFailureHasUnsentProof(t *testing.T) {
	owner := newAttemptOwner(&config.InstanceConfig{InstanceID: "instance", InferenceOwnerFresh: true}, nil)
	prepared, err := owner.prepare(uuid.NewString(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.consume(prepared.AttemptID); err != nil {
		t.Fatal(err)
	}
	ctx := withAttempt(context.Background(), owner, prepared.AttemptID)
	if err := startInferenceCommand(ctx, exec.Command("missing-kandev-owner-fixture-834718.exe")); err == nil {
		t.Fatal("missing executable started")
	}
	status, _ := owner.get(prepared.AttemptID)
	if !status.Known || !status.Settled || status.State != AttemptNotStarted || status.PromptDispatch != PromptNotSent {
		t.Fatalf("start failure not compensated: %+v", status)
	}
}
