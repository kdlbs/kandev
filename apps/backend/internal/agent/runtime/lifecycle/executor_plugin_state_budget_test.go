package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestHostFieldBudgetSumsToTheReserve(t *testing.T) {
	sum := 0
	for _, group := range hostFieldBudget {
		sum += group.max
	}
	if sum > pluginExecutorHostReserveBytes {
		t.Fatalf("host field maxima sum to %d, want at most %d", sum, pluginExecutorHostReserveBytes)
	}
	if pluginExecutorHostReserveBytes+pluginExecutorMaxStateBudget != pluginExecutorInventoryMaxBytes {
		t.Fatalf("reserve + max budget must equal the envelope cap")
	}
}

func TestEveryEnvelopeFieldHasAHostBudgetEntry(t *testing.T) {
	covered := map[string]bool{}
	for _, group := range hostFieldBudget {
		for _, key := range group.keys {
			covered[key] = true
		}
	}
	typ := reflect.TypeOf(pluginExecutorInventory{})
	for i := 0; i < typ.NumField(); i++ {
		key, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if !covered[key] {
			t.Errorf("envelope field %q has no hostFieldBudget entry", key)
		}
	}
}

// worstCaseInventory fills every host-written group to its maximum.
func worstCaseInventory(stateBytes int) pluginExecutorInventory {
	inventory := pluginExecutorInventory{
		PluginID: "p", InstallationID: "i", ProviderKey: "k", ProviderIdentity: "x", Phase: pluginExecutorPhaseReady,
		SupportedStateVersions: []int{1},
		ProfileConfig:          map[string]string{"k": strings.Repeat("a", 2000)},
		SecretReferences:       map[string]string{"s": strings.Repeat("b", 1900)},
		NegotiatedCapabilities: []string{"workspace_descriptor@1", "destroy_checkpoints@1"},
		Resource:               &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "h", StateJson: `{"a":"` + strings.Repeat("&", stateBytes/6-8) + `"}`},
	}
	return inventory
}

func TestWorstCaseEnvelopeFitsTheCapAtDefaultAndMaximumBudget(t *testing.T) {
	for _, budget := range []int{pluginExecutorDefaultStateBudget, pluginExecutorMaxStateBudget} {
		inventory := worstCaseInventory(budget)
		if err := checkPluginExecutorEnvelopeBudget(inventory, budget); err != nil {
			t.Fatalf("budget %d: %v", budget, err)
		}
		data, err := json.Marshal(inventory)
		if err != nil || len(data) > pluginExecutorInventoryMaxBytes {
			t.Fatalf("budget %d: marshaled %d bytes (err %v), want at most %d", budget, len(data), err, pluginExecutorInventoryMaxBytes)
		}
	}
}

func TestStateAboveTheBudgetIsStateTooLarge(t *testing.T) {
	inventory := worstCaseInventory(pluginExecutorDefaultStateBudget)
	inventory.Resource.StateJson = `{"a":"` + strings.Repeat("x", pluginExecutorDefaultStateBudget) + `"}`
	if err := checkPluginExecutorEnvelopeBudget(inventory, pluginExecutorDefaultStateBudget); !errors.Is(err, models.ErrExecutorStateTooLarge) {
		t.Fatalf("err = %v, want state_too_large", err)
	}
}

func TestStateIsMeasuredOnItsEscapedForm(t *testing.T) {
	inventory := worstCaseInventory(pluginExecutorDefaultStateBudget)
	// 1400 ampersands are 1400 raw bytes but 8400 once marshaled into the envelope.
	inventory.Resource.StateJson = `{"a":"` + strings.Repeat("&", 1400) + `"}`
	if err := checkPluginExecutorEnvelopeBudget(inventory, pluginExecutorDefaultStateBudget); !errors.Is(err, models.ErrExecutorStateTooLarge) {
		t.Fatalf("err = %v, want state_too_large on the escaped size", err)
	}
}

func TestOversizedHostFieldIsStateTooLarge(t *testing.T) {
	inventory := worstCaseInventory(1024)
	inventory.ProfileConfig = map[string]string{"k": strings.Repeat("a", 6000)}
	if err := checkPluginExecutorEnvelopeBudget(inventory, pluginExecutorDefaultStateBudget); !errors.Is(err, models.ErrExecutorStateTooLarge) {
		t.Fatalf("err = %v, want state_too_large for the existing-field group", err)
	}
}

func TestNonDeclaringProviderKeepsTheLegacyCapAndError(t *testing.T) {
	inventory := pluginExecutorInventory{Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "h", StateJson: strings.Repeat("a", 20000)}}
	if err := checkPluginExecutorEnvelopeBudget(inventory, 0); err != nil {
		t.Fatalf("err = %v; a record with no negotiated set is not budgeted", err)
	}
}

func TestOversizedProvisionStateIsRefusedAndTheResourceCleaned(t *testing.T) {
	fake := &capabilityConfirmerFake{}
	fake.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "allocated", StateVersion: 1, Platform: "linux-amd64",
		WorkspacePath: "/workspace", WorkspaceIdentity: "workspace-1",
		StateJson: `{"id":"` + strings.Repeat("x", pluginExecutorDefaultStateBudget) + `"}`,
	}}
	fake.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}
	provider := capabilityLaunchProvider()
	runtime := NewPluginRemoteExecutor(fake, newTestLogger())
	runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	request := pluginExecutorLaunchRequest(provider)
	var persisted []pluginExecutorInventory
	request.CheckpointRuntimeInventory = func(_ context.Context, checkpoint map[string]interface{}) error {
		persisted = append(persisted, checkpoint[MetadataKeyPluginExecutor].(pluginExecutorInventory))
		return nil
	}
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }

	_, err := runtime.CreateInstance(context.Background(), request)
	if !errors.Is(err, models.ErrExecutorStateTooLarge) {
		t.Fatalf("err = %v, want state_too_large", err)
	}
	if fake.destroyRequest == nil {
		t.Fatal("the resource created by the refused response was not cleaned up")
	}
	for _, inventory := range persisted {
		if inventory.Resource != nil && len(inventory.Resource.GetStateJson()) > 0 {
			t.Fatalf("phase %s persisted %d bytes of refused provider state", inventory.Phase, len(inventory.Resource.GetStateJson()))
		}
	}
}
