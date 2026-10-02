package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types"
)

const managedStartupEvidenceHelperEnv = "KANDEV_MANAGED_STARTUP_EVIDENCE_HELPER"

func TestManagedStartupEvidenceHelper(t *testing.T) {
	switch os.Getenv(managedStartupEvidenceHelperEnv) {
	case "exit_zero":
		os.Exit(0)
	case "exit_one":
		fmt.Fprintln(os.Stderr, "npm error code ECONNRESET")
		os.Exit(1)
	case "hold":
		select {}
	}
}

func TestManagedStartupEvidence(t *testing.T) {
	t.Run("ordinary zero exit with empty stderr", func(t *testing.T) {
		mgr, generation := startManagedStartupEvidenceHelper(t, "exit_zero")
		evidence := waitManagedStartupEvidence(t, mgr, generation, nil)
		if evidence.ExitDisposition != types.ManagedStartupExitOrdinary || evidence.ExitCode == nil || *evidence.ExitCode != 0 {
			t.Fatalf("exit evidence = %#v, want ordinary exit code 0", evidence)
		}
		if evidence.NPMCode != "" || !evidence.CollectionComplete {
			t.Fatalf("exit evidence = %#v, want no npm code and complete collection", evidence)
		}
	})

	t.Run("ordinary error exit with canonical npm code", func(t *testing.T) {
		mgr, generation := startManagedStartupEvidenceHelper(t, "exit_one")
		evidence := waitManagedStartupEvidence(t, mgr, generation, nil)
		if evidence.ExitDisposition != types.ManagedStartupExitOrdinary || evidence.ExitCode == nil || *evidence.ExitCode != 1 {
			t.Fatalf("exit evidence = %#v, want ordinary exit code 1", evidence)
		}
		if evidence.NPMCode != "ECONNRESET" || !evidence.CollectionComplete {
			t.Fatalf("exit evidence = %#v, want canonical ECONNRESET and complete collection", evidence)
		}
	})

	t.Run("intentional stop", func(t *testing.T) {
		mgr, generation := startManagedStartupEvidenceHelper(t, "exit_one")
		mgr.status.Store(StatusStopping)
		evidence := waitManagedStartupEvidence(t, mgr, generation, nil)
		if evidence.ExitDisposition != types.ManagedStartupExitIntentional || evidence.ExitCode != nil {
			t.Fatalf("exit evidence = %#v, want intentional stop without an exit code", evidence)
		}
	})

	t.Run("generation mismatch", func(t *testing.T) {
		mgr, generation := startManagedStartupEvidenceHelper(t, "exit_zero")
		_ = waitManagedStartupEvidence(t, mgr, generation, nil)
		if got := mgr.ManagedStartupEvidence(context.Background(), generation+1); got != nil {
			t.Fatalf("mismatched generation evidence = %#v, want nil", got)
		}
	})
}

func TestManagedStartupEvidenceSeparatesEmptyUnknownAndIncompleteDiagnostics(t *testing.T) {
	empty := newManagedStartupEvidence(1, nil, false, true, true, nil)
	if !empty.CollectionComplete || !empty.NPMDiagnosticComplete || empty.NPMDiagnosticPresent || empty.UnclassifiedNPMCode {
		t.Fatalf("empty diagnostic evidence = %#v, want complete and genuinely empty", empty)
	}

	unknownLine, keep := safeManagedNpmStderrLine("npm error code EUSAGE")
	if !keep {
		t.Fatal("expected an unknown canonical npm code to retain a safe marker")
	}
	unknown := newManagedStartupEvidence(2, nil, false, true, true, []string{unknownLine})
	if !unknown.NPMDiagnosticPresent || !unknown.NPMDiagnosticComplete || !unknown.UnclassifiedNPMCode || unknown.NPMCode != "" {
		t.Fatalf("unclassified npm diagnostic evidence = %#v, want complete unknown marker without retry code", unknown)
	}

	oversized := newManagedStartupEvidence(3, nil, false, true, true, []string{
		"npm error code EACCES",
		strings.Repeat("diagnostic ", 2_000),
	})
	if !oversized.NPMDiagnosticPresent || oversized.NPMDiagnosticComplete || oversized.NPMCode != "" {
		t.Fatalf("oversized npm diagnostic evidence = %#v, want incomplete and no trusted code", oversized)
	}
}

func TestManagedStartupEvidenceMarksTruncatedStderrRingIncomplete(t *testing.T) {
	mgr := &Manager{}
	for range defaultStderrBufferSize {
		mgr.appendStderr("retained line")
	}
	if _, complete := mgr.managedStartupStderrSnapshot(); !complete {
		t.Fatal("full but untruncated stderr ring should be complete")
	}
	mgr.appendStderr("line that evicts an earlier diagnostic")
	if _, complete := mgr.managedStartupStderrSnapshot(); complete {
		t.Fatal("stderr ring eviction must mark diagnostic retention incomplete")
	}
	mgr.ClearStderrBuffer()
	if lines, complete := mgr.managedStartupStderrSnapshot(); !complete || len(lines) != 0 {
		t.Fatalf("cleared stderr snapshot = %#v, complete=%v, want empty and complete", lines, complete)
	}
}

func startManagedStartupEvidenceHelper(t *testing.T, mode string) (*Manager, uint64) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestManagedStartupEvidenceHelper$")
	cmd.Env = append(os.Environ(), managedStartupEvidenceHelperEnv+"="+mode)
	stderr, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	cmd.Stderr = writer
	mgr := &Manager{
		cmd:       cmd,
		stderr:    stderr,
		logger:    newTestLogger(t),
		doneCh:    make(chan struct{}),
		updatesCh: make(chan adapter.AgentEvent, 1),
		groupAliveFn: func(int) bool {
			return false
		},
	}
	mgr.status.Store(StatusRunning)
	t.Cleanup(func() {
		if cmd.ProcessState == nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = writer.Close()
		_ = stderr.Close()
		mgr.wg.Wait()
	})
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	generation := mgr.beginManagedStartupGeneration()
	if err := writer.Close(); err != nil {
		t.Fatalf("close parent stderr writer: %v", err)
	}
	return mgr, generation
}

func waitManagedStartupEvidence(
	t *testing.T,
	mgr *Manager,
	generation uint64,
	setStopping func(),
) *types.ManagedStartupEvidence {
	t.Helper()
	if setStopping != nil {
		setStopping()
	}
	stderrDone := make(chan struct{})
	mgr.wg.Add(2)
	go mgr.readStderr(stderrDone)
	mgr.waitForExitGeneration(stderrDone, generation)
	mgr.wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	evidence := mgr.ManagedStartupEvidence(ctx, generation)
	if evidence == nil {
		t.Fatal("managed startup evidence was not retained")
	}
	if evidence.ProcessGeneration != generation {
		t.Fatalf("process generation = %d, want %d", evidence.ProcessGeneration, generation)
	}
	return evidence
}
