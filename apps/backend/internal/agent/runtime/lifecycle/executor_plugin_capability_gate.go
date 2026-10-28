package lifecycle

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/plugins/hostcaps"
	"github.com/kandev/kandev/internal/task/models"
)

// PluginExecutorCapabilityConfirmer is implemented by the plugin service. It
// confirms that the installed manifest, the host registry, and the accepted
// echo of the running provider agree before any provider allocation call.
type PluginExecutorCapabilityConfirmer interface {
	ConfirmExecutorProviderCapabilities(ctx context.Context, pluginID, providerKey string, profile models.ExecutorProviderProfile) error
}

// negotiatedCapabilities returns the sorted `name@version` snapshot pinned to
// an environment, or nil for a provider that declares no capability.
func negotiatedCapabilities(provider models.ExecutorProvider) []string {
	if len(provider.RequiredCapabilities) == 0 {
		return nil
	}
	out := make([]string, 0, len(provider.RequiredCapabilities))
	for _, requirement := range provider.RequiredCapabilities {
		out = append(out, requirement.Name+"@"+strconv.Itoa(requirement.Version))
	}
	slices.Sort(out)
	return out
}

// confirmPluginExecutorCapabilities gates one launch on capability agreement.
// A provider declaring nothing is not gated; one declaring capabilities with no
// confirmer available is refused, never admitted unchecked.
func (r *PluginRemoteExecutor) confirmPluginExecutorCapabilities(ctx context.Context, profile models.ExecutorProviderLaunchProfile) error {
	if len(profile.Provider.RequiredCapabilities) == 0 {
		return nil
	}
	confirmer, ok := r.operations.(PluginExecutorCapabilityConfirmer)
	if !ok {
		return fmt.Errorf("%w: capability confirmation is unavailable", models.ErrExecutorCapabilityMismatch)
	}
	return confirmer.ConfirmExecutorProviderCapabilities(ctx, profile.Provider.PluginID, profile.Provider.Key, models.ExecutorProviderProfile{
		ProfileID: profile.ProfileID, Config: profile.Config, SecretValues: profile.SecretValues,
	})
}

// confirmRetainedPluginExecutorCapabilities gates attach on the set recorded
// when the environment was admitted, never on the installed manifest. A
// recorded version the host no longer supports leaves the record retained and
// the provider unavailable for it.
func (r *PluginRemoteExecutor) confirmRetainedPluginExecutorCapabilities(ctx context.Context, inventory pluginExecutorInventory, profile *models.ExecutorProviderLaunchProfile) error {
	if len(inventory.NegotiatedCapabilities) == 0 {
		return nil
	}
	recorded := make([]models.ExecutorProviderCapabilityRequirement, 0, len(inventory.NegotiatedCapabilities))
	for _, pair := range inventory.NegotiatedCapabilities {
		name, versionText, found := strings.Cut(pair, "@")
		version, err := strconv.Atoi(versionText)
		entry, known := hostcaps.Lookup(name)
		if !found || err != nil || !known || version < entry.Min || version > entry.Max {
			return fmt.Errorf("%w: recorded capability is no longer supported", models.ErrExecutorCapabilityMismatch)
		}
		recorded = append(recorded, models.ExecutorProviderCapabilityRequirement{Name: name, Version: version})
	}
	gated := *profile
	gated.Provider.RequiredCapabilities = recorded
	return r.confirmPluginExecutorCapabilities(ctx, gated)
}
