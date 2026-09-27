package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
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

	provider, err := validateExecutorProviderAdmission(true, record, "lambda")
	if err != nil {
		t.Fatalf("validateExecutorProviderAdmission() error = %v", err)
	}
	if provider.Key != "lambda" || ExecutorProviderIdentity(record.ID, provider.Key) != "plugin:example-provider:lambda" {
		t.Fatalf("admitted provider = %+v, identity = %q", provider, ExecutorProviderIdentity(record.ID, provider.Key))
	}

	if _, err := validateExecutorProviderAdmission(true, record, "missing"); !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("undeclared provider error = %v, want unavailable", err)
	}
	if _, err := validateExecutorProviderAdmission(false, record, "lambda"); !errors.Is(err, ErrExecutorProvidersDisabled) {
		t.Fatalf("disabled provider error = %v, want disabled", err)
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

func TestPluginExecutorDisabledNoDispatch(t *testing.T) {
	service := NewService(store.NewFSStore(t.TempDir()), NewRegistry(), nil, nil)
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
	service.SetRemoteExecutorPluginsEnabled(false)
	if err := service.validateExecutorProviderRuntime(context.Background(), record); err != nil {
		t.Fatalf("disabled contract probe = %v, want no-op", err)
	}
	dispatched := false
	err := service.withExecutorProvider(context.Background(), record.ID, "lambda", func(context.Context, *pluginsdk.RemotePlugin, *manifest.ExecutorProvider) error {
		dispatched = true
		return nil
	})
	if !errors.Is(err, ErrExecutorProvidersDisabled) {
		t.Fatalf("withExecutorProvider() error = %v, want disabled", err)
	}
	if dispatched {
		t.Fatal("provider callback ran while remote executor plugins were disabled")
	}
	if _, err := service.Get(record.ID); err != nil {
		t.Fatalf("disabled provider inventory became unreadable: %v", err)
	}
}
