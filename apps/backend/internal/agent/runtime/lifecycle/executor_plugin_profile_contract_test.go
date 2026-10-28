package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestRetainedProfileSizeIsMeasuredOnlyForARecordedNegotiatedSet(t *testing.T) {
	profile := &models.ExecutorProviderLaunchProfile{
		Config:           map[string]string{"name": strings.Repeat("x", 5000)},
		SecretReferences: map[string]string{"credential": "secret-1"},
	}
	declared := pluginExecutorInventory{NegotiatedCapabilities: []string{"typed_profile_values@1"}}
	if err := checkRetainedPluginExecutorProfileSize(declared, profile); !errors.Is(err, models.ErrExecutorProfileTooLarge) {
		t.Fatalf("err = %v, want profile_too_large for a recorded negotiated set", err)
	}
	if err := checkRetainedPluginExecutorProfileSize(pluginExecutorInventory{}, profile); err != nil {
		t.Fatalf("an environment allocated with an empty set must not be re-measured: %v", err)
	}
	small := &models.ExecutorProviderLaunchProfile{Config: map[string]string{"name": "ok"}}
	if err := checkRetainedPluginExecutorProfileSize(declared, small); err != nil {
		t.Fatalf("small profile: %v", err)
	}
}

func typedLaunchProvider(capabilities ...string) models.ExecutorProvider {
	provider := testPluginExecutorLaunchProvider()
	for _, name := range capabilities {
		provider.RequiredCapabilities = append(provider.RequiredCapabilities, models.ExecutorProviderCapabilityRequirement{Name: name, Version: 1})
	}
	provider.ProfileSchema = map[string]any{"type": "object", "properties": map[string]any{
		"region":     map[string]any{"type": "string"},
		"cpus":       map[string]any{"type": "integer"},
		"credential": map[string]any{"type": "string", "secret": true},
	}}
	return provider
}

func provisionWithProfile(t *testing.T, provider models.ExecutorProvider, config map[string]string) (*capabilityConfirmerFake, error) {
	t.Helper()
	fake := &capabilityConfirmerFake{}
	fake.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{}
	runtime := NewPluginRemoteExecutor(fake, newTestLogger())
	runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	request := pluginExecutorLaunchRequest(provider)
	request.PluginExecutor.Profile.Config = config
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	_, err := runtime.CreateInstance(context.Background(), request)
	return fake, err
}

func TestProvisionCarriesTypedConfigAndSecretRefsOnlyWhenNegotiated(t *testing.T) {
	config := map[string]string{"region": "eu-west-1", "cpus": "4"}
	fake, _ := provisionWithProfile(t, typedLaunchProvider("typed_profile_values", "secret_references"), config)
	profile := fake.provisionRequest.GetProfile()
	if profile.GetTypedConfig()["cpus"].GetIntValue() != 4 || profile.GetSecretRefs()["credential"].GetId() != "vault-ref-1" {
		t.Fatalf("typed_config = %v, secret_refs = %v", profile.GetTypedConfig(), profile.GetSecretRefs())
	}
	if profile.GetSecretValues()["credential"] != "launch-secret" || profile.GetConfig()["cpus"] != "4" {
		t.Fatalf("values and canonical config must still be sent: %v %v", profile.GetSecretValues(), profile.GetConfig())
	}

	fake, _ = provisionWithProfile(t, typedLaunchProvider(), config)
	profile = fake.provisionRequest.GetProfile()
	if len(profile.GetTypedConfig()) != 0 || len(profile.GetSecretRefs()) != 0 {
		t.Fatalf("a provider that negotiated neither capability received %v %v", profile.GetTypedConfig(), profile.GetSecretRefs())
	}
}

func TestUnparseableStoredIntegerIsRefusedBeforeAnyProviderCall(t *testing.T) {
	fake, err := provisionWithProfile(t, typedLaunchProvider("typed_profile_values"), map[string]string{"cpus": "many"})
	if !errors.Is(err, models.ErrExecutorProfileInvalidType) {
		t.Fatalf("err = %v, want invalid_type", err)
	}
	if fake.provisionRequest != nil {
		t.Fatal("the provider was called with a profile that does not parse")
	}
	if strings.Contains(err.Error(), "many") {
		t.Fatalf("error echoes the value: %v", err)
	}
}

func provisionReturningState(t *testing.T, stateJSON string, secrets map[string]string) (*capabilityConfirmerFake, []pluginExecutorInventory, error) {
	t.Helper()
	fake := &capabilityConfirmerFake{}
	fake.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "allocated", StateVersion: 1, StateJson: stateJSON,
	}}
	fake.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}
	runtime := NewPluginRemoteExecutor(fake, newTestLogger())
	runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	request.PluginExecutor.Profile.SecretValues = secrets
	var persisted []pluginExecutorInventory
	request.CheckpointRuntimeInventory = func(_ context.Context, checkpoint map[string]interface{}) error {
		persisted = append(persisted, checkpoint[MetadataKeyPluginExecutor].(pluginExecutorInventory))
		return nil
	}
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	_, err := runtime.CreateInstance(context.Background(), request)
	return fake, persisted, err
}

func TestSecretValueInProviderStateIsRejectedAndNeverPersisted(t *testing.T) {
	secret := "launch-secret-value"
	fake, persisted, err := provisionReturningState(t, `{"id":"prefix-`+secret+`-suffix"}`, map[string]string{"credential": secret})
	if !errors.Is(err, models.ErrExecutorSecretInState) {
		t.Fatalf("err = %v, want secret_in_state", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error echoes the secret: %v", err)
	}
	if fake.destroyRequest == nil {
		t.Fatal("the resource created by the rejected response was not cleaned up")
	}
	for _, inventory := range persisted {
		if inventory.Resource != nil && strings.Contains(inventory.Resource.GetStateJson(), secret) {
			t.Fatalf("phase %s persisted the secret in provider state", inventory.Phase)
		}
	}
}

func TestSecretShorterThanEightBytesIsNotScanned(t *testing.T) {
	_, _, err := provisionReturningState(t, `{"id":"abc1234"}`, map[string]string{"credential": "abc1234"})
	if errors.Is(err, models.ErrExecutorSecretInState) {
		t.Fatalf("a 7-byte value must not be scanned: %v", err)
	}
}

func TestSecretInStateCheckMatchesTheEscapedForm(t *testing.T) {
	err := rejectSecretInProviderState(map[string]string{"credential": "pa\"ss\\word-123"},
		&pluginsdk.ExecutorResourceDescriptor{StateJson: `{"id":"pa\"ss\\word-123"}`})
	if !errors.Is(err, models.ErrExecutorSecretInState) {
		t.Fatalf("err = %v, want the JSON-escaped copy detected", err)
	}
	if err := rejectSecretInProviderState(nil, &pluginsdk.ExecutorResourceDescriptor{StateJson: `{"id":"x"}`}); err != nil {
		t.Fatalf("no revealed values: %v", err)
	}
}
