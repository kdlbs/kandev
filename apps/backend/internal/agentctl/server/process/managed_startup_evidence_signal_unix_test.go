//go:build !windows

package process

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedStartupEvidenceSignalExit(t *testing.T) {
	mgr, generation := startManagedStartupEvidenceHelper(t, "hold")
	stderrDone := make(chan struct{})
	waitDone := make(chan struct{})
	mgr.wg.Add(2)
	go mgr.readStderr(stderrDone)
	go func() {
		mgr.waitForExitGeneration(stderrDone, generation)
		close(waitDone)
	}()

	if err := mgr.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send termination signal: %v", err)
	}
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("wait for signal-terminated process timed out")
	}
	mgr.wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	evidence := mgr.ManagedStartupEvidence(ctx, generation)
	if evidence == nil {
		t.Fatal("managed startup evidence was not retained")
	}
	if evidence.ExitDisposition != types.ManagedStartupExitSignal || evidence.ExitCode != nil {
		t.Fatalf("exit evidence = %#v, want signal disposition without exit code", evidence)
	}
	if !evidence.CollectionComplete {
		t.Fatalf("exit evidence = %#v, want complete collection", evidence)
	}
}
