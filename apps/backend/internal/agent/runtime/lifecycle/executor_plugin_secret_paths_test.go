package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kdlbs/kandev/pluginsdk"
)

// Every launch path that holds a provider resource after a refusal must keep a
// revealed profile secret out of the raw stored envelope and out of the handoff.
func TestSecretInStateNeverReachesStorageOnAnyRefusalPath(t *testing.T) {
	paths := []struct {
		name  string
		setup func(*workspaceLaunchHarness)
	}{
		{"provision_rejected", func(h *workspaceLaunchHarness) {
			h.operations.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{
				Resource: secretBearingResource(), Error: &pluginsdk.ExecutorProviderError{Code: "node_contract", MessageId: "executor.node.identity_mismatch"},
			}
		}},
		{"provision_recovered", func(h *workspaceLaunchHarness) {
			h.operations.provisionResponse = nil
			h.operations.provisionErr = errors.New("provision connection lost")
			h.operations.recoverResponse = &pluginsdk.RecoverExecutorOperationResponse{Outcome: "found", Resource: secretBearingResource()}
		}},
		{"provision_recovered_rejected", func(h *workspaceLaunchHarness) {
			invalid := secretBearingResource()
			invalid.StateVersion = 99
			h.operations.provisionResponse = nil
			h.operations.provisionErr = errors.New("provision connection lost")
			h.operations.recoverResponse = &pluginsdk.RecoverExecutorOperationResponse{Outcome: "found", Resource: invalid}
		}},
	}
	for _, path := range paths {
		for _, fenced := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fenced=%v", path.name, fenced), func(t *testing.T) {
				harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), secretBearingResource())
				path.setup(harness)
				var stored []string
				record := func(metadata map[string]interface{}) {
					encoded, err := json.Marshal(metadata)
					if err != nil {
						t.Fatal(err)
					}
					stored = append(stored, string(encoded))
				}
				harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
					record(metadata)
					return nil
				}
				harness.request.RecordUnrecordedRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
					record(metadata)
					return nil
				}
				if fenced {
					harness.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{claimErr: fmt.Errorf("%w: moved on", repoerrors.ErrTaskEnvironmentOwnershipChanged)})
					harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
						if encoded, _ := json.Marshal(metadata); strings.Contains(string(encoded), "opaque-resource") {
							return errors.New("store unavailable")
						}
						record(metadata)
						return nil
					}
				}

				if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err == nil {
					t.Fatal("a refused launch succeeded")
				}
				for _, raw := range stored {
					if strings.Contains(raw, leakedProfileSecret) {
						t.Fatalf("stored envelope carries a revealed secret: %s", raw)
					}
				}
			})
		}
	}
}
