package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedStartupRecoveryClassification(t *testing.T) {
	exitCode := 1
	cases := []struct {
		name              string
		phase             SessionInitializationPhase
		evidence          *agentctltypes.ManagedStartupEvidence
		stderr            []string
		stderrConfigured  bool
		advanceGeneration bool
		wantHandled       bool
		wantOnline        bool
		terminal          bool
	}{
		{
			name:  "transient npm code retries online",
			phase: SessionInitializationPhaseACPInitialize,
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
				ExitCode: &exitCode, NPMCode: "ECONNRESET", CollectionComplete: true,
				NPMDiagnosticPresent: true, NPMDiagnosticComplete: true,
			},
			stderr: []string{"npm error code ECONNRESET"}, wantHandled: true, wantOnline: true,
		},
		{
			name:  "ordinary exit with empty stderr retries unchanged",
			phase: SessionInitializationPhaseACPInitialize,
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
				ExitCode: &exitCode, CollectionComplete: true, NPMDiagnosticComplete: true,
			},
			stderrConfigured: true, wantHandled: true,
		},
		{
			name:  "legacy exact top-level ETARGET retries online",
			phase: SessionInitializationPhaseACPInitialize,
			stderr: []string{
				"npm error code ETARGET",
				"npm error notarget No matching version found for opencode-ai@1.2.3",
			},
			wantHandled: true, wantOnline: true,
		},
		{
			name:             "legacy missing evidence and empty stderr do not retry",
			phase:            SessionInitializationPhaseACPInitialize,
			stderrConfigured: true,
		},
		{
			name:  "generation mismatch does not retry",
			phase: SessionInitializationPhaseACPInitialize,
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
				CollectionComplete:    true,
				NPMDiagnosticComplete: true,
			},
			stderrConfigured: true, advanceGeneration: true,
		},
		{
			name:  "incomplete evidence does not retry",
			phase: SessionInitializationPhaseACPInitialize,
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
			},
			stderrConfigured: true,
		},
		{
			name:  "signal exit does not retry",
			phase: SessionInitializationPhaseACPInitialize,
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitSignal,
				CollectionComplete:    true,
				NPMDiagnosticComplete: true,
			},
			stderrConfigured: true,
		},
		{
			name:  "intentional stop does not retry",
			phase: SessionInitializationPhaseACPInitialize,
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitIntentional,
				CollectionComplete:    true,
				NPMDiagnosticComplete: true,
			},
			stderrConfigured: true,
		},
		{
			name:  "permanent npm error does not retry",
			phase: SessionInitializationPhaseACPInitialize,
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
				NPMCode: "E401", CollectionComplete: true,
				NPMDiagnosticPresent: true, NPMDiagnosticComplete: true,
			},
			stderr:      []string{"npm error code E401"},
			wantHandled: true, terminal: true,
		},
		{
			name: "session creation error does not retry",
			stderr: []string{
				"npm error code ETARGET",
				"npm error notarget No matching version found for opencode-ai@1.2.3",
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			client, release := execution.AcquireAgentCtlClient()
			if _, err := client.Start(context.Background()); err != nil {
				release()
				t.Fatalf("seed process generation: %v", err)
			}
			if tt.advanceGeneration {
				if _, err := client.Start(context.Background()); err != nil {
					release()
					t.Fatalf("advance process generation: %v", err)
				}
			}
			release()
			mock.mu.Lock()
			mock.httpActions = nil
			mock.stderrLines = append([]string(nil), tt.stderr...)
			mock.stderrConfigured = tt.stderrConfigured
			mock.mu.Unlock()

			initErr := errors.New("session/new failed")
			if tt.phase != "" {
				var cause error = &agentctl.InitializeError{Message: "ACP initialize failed", StartupEvidence: tt.evidence}
				initErr = &SessionInitializationPhaseError{Phase: tt.phase, Cause: cause}
			}
			retry, got := mgr.prepareManagedRuntimeStartupRetry(context.Background(), execution, initErr, agentConfig)
			if got != tt.wantHandled {
				t.Fatalf("handled = %v, want %v (retry=%#v)", got, tt.wantHandled, retry)
			}
			if !got {
				return
			}
			if retry.terminal != tt.terminal {
				t.Fatalf("terminal = %v, want %v", retry.terminal, tt.terminal)
			}
			if tt.terminal {
				if len(retry.retryArgs) != 0 {
					t.Fatalf("terminal failure unexpectedly has retry args: %#v", retry.retryArgs)
				}
				return
			}
			wantArgs := execution.AgentArgs
			if tt.wantOnline {
				wantArgs = append([]string(nil), execution.AgentArgs...)
				for i, arg := range wantArgs {
					if arg == managedRuntimePreferOfflineArg {
						wantArgs[i] = "--prefer-online"
					}
				}
			}
			if !reflect.DeepEqual(retry.retryArgs, wantArgs) {
				t.Fatalf("retry args = %#v, want %#v", retry.retryArgs, wantArgs)
			}
		})
	}
}
