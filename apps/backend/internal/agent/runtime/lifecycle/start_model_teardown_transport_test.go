package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

type startModelErrorBranch struct {
	name   string
	policy StartModelPolicy
	state  *CachedModelState
}

func startModelErrorBranches() []startModelErrorBranch {
	return []startModelErrorBranch{
		{"exact requested model", StartModelPolicy{Model: "saved-model", RequireExactModel: true}, modelState("saved-model")},
		{"compatible requested model", StartModelPolicy{Model: "saved-model"}, modelState("saved-model")},
		{"unique variation", StartModelPolicy{Model: "saved-model"}, modelState("saved-model[1m]")},
		{"advertised fallback", StartModelPolicy{Model: "saved-model", FallbackModel: "alternate"}, modelState("alternate")},
	}
}

// Typed loss of the backend stream is transport evidence independent of any
// provider-supplied diagnostic. Caller cancellation and deadlines stay intact.
func TestStartModelTransportFailureIsNotReportedAsModelApplicationFailure(t *testing.T) {
	transportErrors := map[string]error{
		"stream not connected": fmt.Errorf("set model request failed: %w", agentctl.ErrAgentStreamNotConnected),
		"cancellation":         context.Canceled,
		"deadline":             context.DeadlineExceeded,
	}
	for _, branch := range startModelErrorBranches() {
		for name, transportErr := range transportErrors {
			t.Run(branch.name+"/"+name, func(t *testing.T) {
				_, err := applyStartModelPolicy(context.Background(), newPolicyTestLogger(), &fakeModelApplier{errs: []error{transportErr}}, branch.state, branch.policy)
				if err != transportErr {
					t.Fatalf("error = %v, want the original transport error unchanged", err)
				}
				var failure *BootstrapFailure
				if errors.As(err, &failure) {
					t.Fatalf("transport failure acquired model evidence: %+v", failure)
				}
			})
		}
	}
}

// Transport wording in an opaque provider reply cannot establish that the
// backend or ACP connection was lost.
func TestStartModelProviderRejectionStaysModelApplicationFailure(t *testing.T) {
	for _, branch := range startModelErrorBranches() {
		for _, message := range []string{"provider rejected selection", "peer disconnected", "connection closed", "notification queue overflow", "agent stream not connected", "context deadline exceeded"} {
			t.Run(branch.name+"/"+message, func(t *testing.T) {
				providerErr := errors.New(message)
				_, err := applyStartModelPolicy(context.Background(), newPolicyTestLogger(), &fakeModelApplier{errs: []error{providerErr}}, branch.state, branch.policy)
				var failure *BootstrapFailure
				if !errors.As(err, &failure) || failure.Reason != models.AgentErrorCauseReasonApplicationFailed {
					t.Fatalf("error = %v, want application_failed bootstrap evidence", err)
				}
				if !errors.Is(err, providerErr) {
					t.Fatal("provider error identity was lost")
				}
			})
		}
	}
}
