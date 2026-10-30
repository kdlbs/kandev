package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type executorProviderProbe struct {
	request  *pluginsdk.ValidateExecutorProfileRequest
	response *pluginsdk.ValidateExecutorProfileResponse
	err      error
}

func (p *executorProviderProbe) ValidateExecutorProfile(_ context.Context, request *pluginsdk.ValidateExecutorProfileRequest) (*pluginsdk.ValidateExecutorProfileResponse, error) {
	p.request = request
	return p.response, p.err
}

func TestPluginExecutorAdmission(t *testing.T) {
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID:           "example-provider",
			Capabilities: manifest.Capabilities{ExecutorProvider: true},
			ExecutorProviders: []manifest.ExecutorProvider{{
				Key:             "lambda",
				ContractVersion: manifest.CurrentExecutorProviderContractVersion,
			}},
		},
		Status: StatusActive,
	}

	provider, err := validateExecutorProviderAdmission(record, "lambda")
	if err != nil {
		t.Fatalf("validateExecutorProviderAdmission() error = %v", err)
	}
	if provider.Key != "lambda" || ExecutorProviderIdentity(record.ID, provider.Key) != "plugin:example-provider:lambda" {
		t.Fatalf("admitted provider = %+v, identity = %q", provider, ExecutorProviderIdentity(record.ID, provider.Key))
	}

	if _, err := validateExecutorProviderAdmission(record, "missing"); !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("undeclared provider error = %v, want unavailable", err)
	}
	disabledRecord := *record
	disabledRecord.Status = StatusDisabled
	if _, err := validateExecutorProviderAdmission(&disabledRecord, "lambda"); !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("disabled plugin admission error = %v, want unavailable", err)
	}
}

func TestPluginExecutorDispatchRejectsWhenProcessIsNotRunning(t *testing.T) {
	service, _, _ := newTestService(t)
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID:           "example-provider",
			Capabilities: manifest.Capabilities{ExecutorProvider: true},
			ExecutorProviders: []manifest.ExecutorProvider{{
				Key:             "lambda",
				ContractVersion: manifest.CurrentExecutorProviderContractVersion,
			}},
		},
		Status: StatusActive,
	}
	service.registry.Add(record)
	called := false
	err := service.withExecutorProvider(context.Background(), record.ID, "lambda", func(context.Context, *pluginsdk.RemotePlugin, *manifest.ExecutorProvider) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("withExecutorProvider() error = %v, want unavailable", err)
	}
	if called {
		t.Fatal("provider callback ran without an active plugin process")
	}
}

func TestPluginExecutorContractProbeRequiresCompleteProvider(t *testing.T) {
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID: "example-provider", Capabilities: manifest.Capabilities{ExecutorProvider: true},
			ExecutorProviders: []manifest.ExecutorProvider{{Key: "lambda", ContractVersion: manifest.CurrentExecutorProviderContractVersion}},
		},
		InstallationID: "installation-1",
	}
	probe := &executorProviderProbe{response: &pluginsdk.ValidateExecutorProfileResponse{
		FieldErrors: []*pluginsdk.ExecutorProviderFieldError{{Field: "region", Code: "required", MessageId: "profile.region.required"}},
	}}
	if err := validateExecutorProviderContracts(context.Background(), record, probe); err != nil {
		t.Fatalf("validateExecutorProviderContracts() error = %v", err)
	}
	if probe.request.GetContext().GetPluginId() != record.ID || probe.request.GetContext().GetInstallationId() != record.InstallationID || probe.request.GetContext().GetProviderKey() != "lambda" {
		t.Fatalf("provider contract probe context = %+v", probe.request.GetContext())
	}

	probe.err = status.Error(codes.Unimplemented, "unsupported")
	if err := validateExecutorProviderContracts(context.Background(), record, probe); !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("missing provider contract error = %v, want unavailable", err)
	}
}

func TestContractProbeSendsTheRegistryAndPassesEachResponseToTheObserver(t *testing.T) {
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID: "example-provider", Capabilities: manifest.Capabilities{ExecutorProvider: true},
			ExecutorProviders: []manifest.ExecutorProvider{{
				Key: "lambda", ContractVersion: manifest.CurrentExecutorProviderContractVersion,
				RequiresHostCapabilities: []manifest.HostCapabilityRequirement{{Name: "workspace_descriptor", Version: 1}},
			}},
		},
		InstallationID: "installation-1",
	}
	probe := &executorProviderProbe{response: &pluginsdk.ValidateExecutorProfileResponse{}}
	gate := &capabilityGate{}
	observe := func(provider *manifest.ExecutorProvider, response *pluginsdk.ValidateExecutorProfileResponse) error {
		return gate.observe(capabilityGateKey{installationID: "installation-1", providerKey: provider.Key, process: probe},
			provider.HostCapabilityRequirements(), response)
	}
	err := validateExecutorProviderContractsObserving(context.Background(), record, probe, observe)
	if !errors.Is(err, ErrExecutorCapabilityMismatch) {
		t.Fatalf("a provider that requires a capability and echoes none must be rejected at activation, got %v", err)
	}
	if len(probe.request.GetHostCapabilities()) != 17 {
		t.Fatalf("probe carried %d registry rows, want 17", len(probe.request.GetHostCapabilities()))
	}
	probe.response = &pluginsdk.ValidateExecutorProfileResponse{
		ImplementedCapabilities: []*pluginsdk.ExecutorCapabilityVersion{{Name: "workspace_descriptor", Version: 1}},
	}
	gate = &capabilityGate{}
	if err := validateExecutorProviderContractsObserving(context.Background(), record, probe, observe); err != nil {
		t.Fatalf("a matching echo must pass: %v", err)
	}
}
