package backendapp

import (
	"context"
	"runtime"
	"slices"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func checkSandboxInstallation(ctx context.Context, providers *plugins.Service, resolved *sandbox.Resolved) error {
	binding, err := sandboxInstallation(ctx, providers, resolved)
	if err != nil {
		return err
	}
	request := &pluginsdk.SandboxRequestContext{ProtocolMajor: 1, InstallationId: binding.InstallationID,
		ProviderKey: binding.ProviderKey, OperationId: uuid.NewString(), AttemptId: uuid.NewString(),
		ExecutionId: uuid.NewString(), Generation: 1}
	described, err := providers.DescribeSandboxProvider(ctx, binding, &pluginsdk.DescribeSandboxProviderRequest{
		Context: request, Runtime: &pluginsdk.SandboxRuntimeFacts{Os: runtime.GOOS, Architecture: runtime.GOARCH, Transport: "local"},
	})
	if err != nil {
		return err
	}
	if !slices.Contains(described.Description.SupportedTransports, "local") {
		return sandbox.ErrIneligible
	}
	if _, err = providers.ReadSandboxRuntime(ctx, binding, described.Description.RuntimeArtifact); err != nil {
		return err
	}
	current, err := sandboxInstallation(ctx, providers, resolved)
	if err != nil {
		return err
	}
	if current != binding {
		return sandbox.ErrConflict
	}
	return nil
}

func sandboxInstallation(ctx context.Context, providers *plugins.Service, resolved *sandbox.Resolved) (plugins.SandboxProviderBinding, error) {
	if resolved == nil || resolved.Revision == nil {
		return plugins.SandboxProviderBinding{}, sandbox.ErrIneligible
	}
	items, err := providers.ListSandboxProviders(ctx)
	if err != nil {
		return plugins.SandboxProviderBinding{}, err
	}
	for _, item := range items {
		if item.PluginRunning && item.Binding.InstallationID == resolved.Revision.Policy.InstallationID && item.Binding.ProviderKey == resolved.Revision.Policy.ProviderKey {
			return item.Binding, nil
		}
	}
	return plugins.SandboxProviderBinding{}, plugins.ErrSandboxProviderUnavailable
}
