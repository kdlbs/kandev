package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedStartupMalformedNPMWireEvidenceCannotAuthorizeRetry(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
	}{
		{
			name: "present flag is a string",
			raw:  `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":true,"npm_diagnostic_present":"yes"}`,
		},
		{
			name: "unclassified flag is a string",
			raw:  `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":true,"unclassified_npm_code":"no"}`,
		},
		{
			name: "npm code is a number",
			raw:  `{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":1,"collection_complete":true,"npm_diagnostic_complete":true,"npm_code":17}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			client, release := execution.AcquireAgentCtlClient()
			_, startErr := client.Start(context.Background())
			processGeneration := client.ProcessGeneration()
			release()
			if startErr != nil {
				t.Fatalf("seed process generation: %v", startErr)
			}
			mock.mu.Lock()
			mock.stderrConfigured = true
			mock.stderrLines = nil
			mock.mu.Unlock()

			var evidence agentctltypes.ManagedStartupEvidence
			if err := json.Unmarshal([]byte(tt.raw), &evidence); err != nil {
				t.Fatalf("decode wire evidence: %v", err)
			}
			evidence.ProcessGeneration = processGeneration
			if evidence.NPMDiagnosticComplete {
				t.Fatalf("wire evidence = %#v, malformed npm fields must invalidate completeness", evidence)
			}
			initErr := &SessionInitializationPhaseError{
				Phase:             SessionInitializationPhaseACPInitialize,
				ProcessGeneration: processGeneration,
				Cause:             &agentctl.InitializeError{Message: "peer disconnected", StartupEvidence: &evidence},
			}
			retry, handled := mgr.prepareManagedRuntimeStartupRetry(context.Background(), execution, initErr, agentConfig)
			if handled || retry != nil {
				t.Fatalf("malformed npm evidence authorized retry: %#v", retry)
			}
		})
	}
}

func TestManagedStartupValidEmptyWireEvidenceKeepsSingleRetry(t *testing.T) {
	mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
	client, release := execution.AcquireAgentCtlClient()
	_, startErr := client.Start(context.Background())
	processGeneration := client.ProcessGeneration()
	release()
	if startErr != nil {
		t.Fatalf("seed process generation: %v", startErr)
	}
	mock.mu.Lock()
	mock.stderrConfigured = true
	mock.stderrLines = nil
	mock.mu.Unlock()
	var evidence agentctltypes.ManagedStartupEvidence
	if err := json.Unmarshal([]byte(`{"process_generation":1,"exit_disposition":"ordinary_exit","exit_code":0,"collection_complete":true,"npm_diagnostic_complete":true}`), &evidence); err != nil {
		t.Fatalf("decode wire evidence: %v", err)
	}
	evidence.ProcessGeneration = processGeneration
	initErr := &SessionInitializationPhaseError{
		Phase:             SessionInitializationPhaseACPInitialize,
		ProcessGeneration: processGeneration,
		Cause:             &agentctl.InitializeError{Message: "peer disconnected", StartupEvidence: &evidence},
	}
	retry, handled := mgr.prepareManagedRuntimeStartupRetry(context.Background(), execution, initErr, agentConfig)
	if !handled || retry == nil || retry.failureReason != "early_exit" {
		t.Fatalf("valid empty evidence retry = %#v, handled=%v", retry, handled)
	}
}
