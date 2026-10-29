package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/plugins"
)

func configureSandboxProviderCatalog(policies *sandbox.Service, providers *plugins.Service) {
	if providers == nil {
		return
	}
	policies.SetAvailabilityCheck(func(ctx context.Context, resolved *sandbox.Resolved) error {
		return checkSandboxInstallation(ctx, providers, resolved)
	})
	policies.SetProviderCatalog(func(ctx context.Context) ([]sandbox.ProviderView, error) {
		descriptors, err := providers.ListSandboxProviders(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]sandbox.ProviderView, 0, len(descriptors))
		for _, descriptor := range descriptors {
			result = append(result, sandbox.ProviderView{
				InstallationID: descriptor.Binding.InstallationID, ProviderKey: descriptor.Binding.ProviderKey,
				DisplayName: descriptor.DisplayName, Version: descriptor.Binding.Version, Available: descriptor.PluginRunning,
			})
		}
		return result, nil
	})
}
