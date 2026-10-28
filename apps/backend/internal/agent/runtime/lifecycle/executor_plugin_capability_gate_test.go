package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

type capabilityConfirmerFake struct {
	pluginExecutorOperationsFake
	calls   int
	err     error
	onCall  func()
	profile models.ExecutorProviderProfile
}

func (f *capabilityConfirmerFake) ConfirmExecutorProviderCapabilities(_ context.Context, _, _ string, profile models.ExecutorProviderProfile) error {
	f.calls++
	f.profile = profile
	if f.onCall != nil {
		f.onCall()
	}
	return f.err
}

func capabilityLaunchProvider() models.ExecutorProvider {
	provider := testPluginExecutorLaunchProvider()
	provider.RequiredCapabilities = []models.ExecutorProviderCapabilityRequirement{
		{Name: "workspace_descriptor", Version: 1}, {Name: "destroy_checkpoints", Version: 1},
	}
	return provider
}

func TestAllocationGateRunsBeforeTheFirstProviderCall(t *testing.T) {
	fake := &capabilityConfirmerFake{}
	order := []string{}
	fake.onCall = func() { order = append(order, "confirm") }
	fake.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{}
	runtime := NewPluginRemoteExecutor(fake, newTestLogger())
	request := pluginExecutorLaunchRequest(capabilityLaunchProvider())
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }

	launch, err := runtime.preparePluginExecutorLaunch(context.Background(), request)
	if err != nil {
		t.Fatalf("preparePluginExecutorLaunch(): %v", err)
	}
	if fake.calls != 1 || fake.provisionRequest != nil {
		t.Fatalf("confirm calls = %d, provision request = %v; want one confirmation and no provider call", fake.calls, fake.provisionRequest)
	}
	want := []string{"destroy_checkpoints@1", "workspace_descriptor@1"}
	if !slices.Equal(launch.inventory.NegotiatedCapabilities, want) {
		t.Fatalf("negotiated = %v, want sorted %v", launch.inventory.NegotiatedCapabilities, want)
	}
	if fake.profile.ProfileID != request.PluginExecutor.Profile.ProfileID {
		t.Fatalf("confirmed profile = %q", fake.profile.ProfileID)
	}
}

func TestAllocationGateMismatchRefusesBeforeAllocation(t *testing.T) {
	fake := &capabilityConfirmerFake{err: models.ErrExecutorCapabilityMismatch}
	runtime := NewPluginRemoteExecutor(fake, newTestLogger())
	var checkpoints int
	request := pluginExecutorLaunchRequest(capabilityLaunchProvider())
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { checkpoints++; return nil }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }

	_, err := runtime.preparePluginExecutorLaunch(context.Background(), request)
	if !errors.Is(err, models.ErrExecutorCapabilityMismatch) {
		t.Fatalf("error = %v, want capability_mismatch", err)
	}
	if checkpoints != 0 || fake.provisionRequest != nil {
		t.Fatalf("checkpoints = %d, provision = %v; a refused gate must record and allocate nothing", checkpoints, fake.provisionRequest)
	}
}

func TestAllocationGateSkippedWhenNothingIsDeclared(t *testing.T) {
	fake := &capabilityConfirmerFake{err: errors.New("must not be called")}
	runtime := NewPluginRemoteExecutor(fake, newTestLogger())
	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }

	launch, err := runtime.preparePluginExecutorLaunch(context.Background(), request)
	if err != nil || fake.calls != 0 || len(launch.inventory.NegotiatedCapabilities) != 0 {
		t.Fatalf("err = %v, calls = %d, negotiated = %v; a non-declaring provider is not gated", err, fake.calls, launch.inventory.NegotiatedCapabilities)
	}
}

func TestAllocationGateFailsClosedWithoutAConfirmer(t *testing.T) {
	runtime := NewPluginRemoteExecutor(&pluginExecutorOperationsFake{}, newTestLogger())
	request := pluginExecutorLaunchRequest(capabilityLaunchProvider())
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }

	if _, err := runtime.preparePluginExecutorLaunch(context.Background(), request); !errors.Is(err, models.ErrExecutorCapabilityMismatch) {
		t.Fatalf("error = %v, want fail closed with capability_mismatch", err)
	}
}

func TestLegacyInventoryDecodesAsTheEmptyNegotiatedSet(t *testing.T) {
	inventory, err := decodePluginExecutorInventory(map[string]interface{}{MetadataKeyPluginExecutor: map[string]interface{}{
		"plugin_id": "example", "installation_id": "install-1", "provider_key": "remote",
		"capabilities": map[string]interface{}{"terminal": true}, "state_version": 1,
	}})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(inventory.NegotiatedCapabilities) != 0 {
		t.Fatalf("negotiated = %v, want empty for a record written before negotiation", inventory.NegotiatedCapabilities)
	}
}

func TestRetainedAttachGateUsesTheRecordedNegotiatedSet(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	inventory := pluginExecutorInventory{NegotiatedCapabilities: []string{"destroy_checkpoints@1"}}

	fake := &capabilityConfirmerFake{err: models.ErrExecutorCapabilityMismatch}
	runtime := NewPluginRemoteExecutor(fake, newTestLogger())
	profile := &models.ExecutorProviderLaunchProfile{Provider: provider, ProfileID: "profile-1"}
	if err := runtime.confirmRetainedPluginExecutorCapabilities(context.Background(), inventory, profile); !errors.Is(err, models.ErrExecutorCapabilityMismatch) || fake.calls != 1 {
		t.Fatalf("err = %v, calls = %d; a recorded set must be confirmed even when the installed manifest lists none", err, fake.calls)
	}

	fake = &capabilityConfirmerFake{err: errors.New("must not be called")}
	runtime = NewPluginRemoteExecutor(fake, newTestLogger())
	if err := runtime.confirmRetainedPluginExecutorCapabilities(context.Background(), pluginExecutorInventory{}, profile); err != nil || fake.calls != 0 {
		t.Fatalf("err = %v, calls = %d; a record without a negotiated set keeps working ungated", err, fake.calls)
	}
}

func TestRetainedAttachRefusesARecordedVersionTheHostNoLongerSupports(t *testing.T) {
	fake := &capabilityConfirmerFake{}
	runtime := NewPluginRemoteExecutor(fake, newTestLogger())
	profile := &models.ExecutorProviderLaunchProfile{Provider: testPluginExecutorLaunchProvider()}
	for _, recorded := range []string{"destroy_checkpoints@9", "unknown_capability@1", "garbage"} {
		inventory := pluginExecutorInventory{NegotiatedCapabilities: []string{recorded}}
		if err := runtime.confirmRetainedPluginExecutorCapabilities(context.Background(), inventory, profile); !errors.Is(err, models.ErrExecutorCapabilityMismatch) {
			t.Fatalf("%s: err = %v, want capability_mismatch", recorded, err)
		}
	}
	if fake.calls != 0 {
		t.Fatalf("confirm calls = %d; an unsupported recorded version fails before any confirmation", fake.calls)
	}
}
