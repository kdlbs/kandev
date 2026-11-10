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
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/startupdiag"
	"github.com/stretchr/testify/require"
)

const startupProcessHelperEnv = "KANDEV_STARTUP_PROCESS_DIAGNOSTIC_HELPER"

func TestStartupProcessDiagnosticHelper(t *testing.T) {
	switch os.Getenv(startupProcessHelperEnv) {
	case "mise":
		fmt.Fprintln(os.Stderr, "mise ERROR Config files in /private/workspace/mise.toml are not trusted.")
		os.Exit(1)
	case "unknown":
		fmt.Fprintln(os.Stderr, "private startup failure at /private/workspace")
		os.Exit(3)
	}
}

func TestStartupProcessDiagnosticMiseTrustProjection(t *testing.T) {
	for _, wrapper := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrapper=%v", wrapper), func(t *testing.T) {
			mgr, generation := startStartupProcessHelper(t, "mise", wrapper)
			evidence := waitStartupProcessEvidence(t, mgr, generation)
			require.Equal(t, types.ManagedStartupExitOrdinary, evidence.ExitDisposition)
			require.NotNil(t, evidence.ExitCode)
			require.Equal(t, 1, *evidence.ExitCode)
			require.Equal(t, startupdiag.ReasonMiseConfigUntrusted, evidence.DiagnosticReason)
			require.True(t, evidence.DiagnosticComplete)
			require.False(t, evidence.NPMDiagnosticComplete)
			require.False(t, evidence.NPMDiagnosticPresent)
			require.NotContains(t, strings.Join(mgr.GetRecentStderr(), "\n"), "/private/workspace")
			require.Contains(t, strings.Join(mgr.GetRecentStderr(), "\n"), startupdiag.MiseConfigUntrustedDetail)
		})
	}
}

func TestStartupProcessDiagnosticUnknownExitKeepsStatusOnly(t *testing.T) {
	for _, wrapper := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrapper=%v", wrapper), func(t *testing.T) {
			mgr, generation := startStartupProcessHelper(t, "unknown", wrapper)
			evidence := waitStartupProcessEvidence(t, mgr, generation)
			require.Equal(t, types.ManagedStartupExitOrdinary, evidence.ExitDisposition)
			require.NotNil(t, evidence.ExitCode)
			require.Equal(t, 3, *evidence.ExitCode)
			require.Empty(t, evidence.DiagnosticReason)
		})
	}
}

func TestStartupProcessDiagnosticIncompleteRetentionDropsReason(t *testing.T) {
	mgr := &Manager{}
	mgr.recordStartupDiagnostic(startupdiag.ReasonMiseConfigUntrusted)
	for range defaultStderrBufferSize {
		mgr.appendStderr("retained line")
	}
	mgr.appendStderr("line that evicts the diagnostic")
	reason, retained := mgr.managedStartupDiagnosticSnapshot()
	require.Equal(t, startupdiag.ReasonMiseConfigUntrusted, reason)
	require.False(t, retained)

	evidence := newManagedStartupEvidence(4, nil, false, true, false, true, []string{"retained"}, reason, retained)
	require.Empty(t, evidence.DiagnosticReason)
	require.False(t, evidence.DiagnosticComplete)
}

func TestStartupProcessDiagnosticZeroExitIsDistinctFromMissingStatus(t *testing.T) {
	zero := newManagedStartupEvidence(1, nil, false, true, true, false, nil, "", true)
	require.NotNil(t, zero.ExitCode)
	require.Equal(t, 0, *zero.ExitCode)

	// Intentional stop intentionally has no status; it must not be represented
	// as an ordinary exit zero.
	missing := newManagedStartupEvidence(1, nil, true, true, true, false, nil, "", true)
	require.Nil(t, missing.ExitCode)
	require.NotEqual(t, types.ManagedStartupExitOrdinary, missing.ExitDisposition)
}

func startStartupProcessHelper(t *testing.T, mode string, wrapper bool) (*Manager, uint64) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupProcessDiagnosticHelper$")
	cmd.Env = append(os.Environ(), startupProcessHelperEnv+"="+mode)
	stderr, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	cmd.Stderr = writer
	cfg := &config.InstanceConfig{}
	if wrapper {
		cfg.LaunchWrapper = launchwrapper.Profile([]string{"wrapper"})
	}
	mgr := &Manager{
		cmd:       cmd,
		stderr:    stderr,
		cfg:       cfg,
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

func waitStartupProcessEvidence(t *testing.T, mgr *Manager, generation uint64) *types.ManagedStartupEvidence {
	t.Helper()
	stderrDone := make(chan stderrReadResult, 1)
	mgr.wg.Add(2)
	go mgr.readStderr(stderrDone)
	mgr.waitForExitGeneration(stderrDone, generation)
	mgr.wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	evidence := mgr.ManagedStartupEvidence(ctx, generation)
	if evidence == nil {
		t.Fatal("startup evidence was not retained")
	}
	return evidence
}
