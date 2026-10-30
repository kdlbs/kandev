package lifecycle

import (
	"context"
	"errors"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/runtimeprobe"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	hostAPIProbeAttempts        = 2
	hostAPIProbeInterval        = 2 * time.Second
	hostAPIProbeBudget          = 10 * time.Second
	cleanupReasonAPIUnreachable = "api_unreachable"
)

// SetHostAPIProbe supplies the backend's own API base URL and the registry
// that issues probe tokens. A provider that forwards the API URL cannot launch
// without both.
func (r *PluginRemoteExecutor) SetHostAPIProbe(hostAPIURL string, probes *runtimeprobe.Registry) {
	r.hostAPIURL = hostAPIURL
	r.hostAPIProbes = probes
}

func pluginExecutorForwardsAPI(provider models.ExecutorProvider) bool {
	return provider.RuntimeAPIURL == manifest.RuntimeAPIURLProviderForwarded
}

// resolvePluginExecutorAPIEndpoints returns the worker-facing API URL for a
// host_public provider, or the host API base URL for a provider_forwarded one.
func (r *PluginRemoteExecutor) resolvePluginExecutorAPIEndpoints(provider models.ExecutorProvider, env map[string]string) (apiURL, hostAPIURL string, err error) {
	if !pluginExecutorForwardsAPI(provider) {
		apiURL, err = validatePluginRuntimeAPIURL(env[envKeyKandevAPIURL])
		return apiURL, "", err
	}
	if r.hostAPIURL == "" || r.hostAPIProbes == nil {
		return "", "", errors.New("plugin executor Kandev API base URL is unavailable")
	}
	return "", r.hostAPIURL, nil
}

// probeForwardedHostAPI proves that the worker reaches the host API through
// the provider's forward. A request that matched on the host counts as success
// even when its reply was lost, so a retry never fails on a spent token.
func (r *PluginRemoteExecutor) probeForwardedHostAPI(ctx context.Context, launch *pluginExecutorLaunchState, control *agentctl.ControlClient) error {
	ctx, cancel := context.WithTimeout(ctx, hostAPIProbeBudget)
	defer cancel()
	probe := r.hostAPIProbes.Begin(launch.request.TaskEnvironmentID)
	defer probe.End()
	for attempt := 1; ; attempt++ {
		err := control.ProbeHostAPI(ctx, launch.probeBaseURL(), probe.Token())
		if err == nil || probe.Reached() {
			return nil
		}
		if attempt == hostAPIProbeAttempts {
			return errors.New("worker cannot reach the Kandev API")
		}
		timer := time.NewTimer(r.hostAPIProbeInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.New("worker cannot reach the Kandev API")
		case <-timer.C:
		}
	}
}

// launchCleanupReason is the destroy reason of a launch rollback: the host API
// probe names its own, every other failed stage reports the generic launch
// reason.
func launchCleanupReason(stage string) string {
	switch stage {
	case cleanupReasonAPIUnreachable:
		return cleanupReasonAPIUnreachable
	case cleanupStageRepositoryInventory:
		return cleanupReasonRepositoryInventoryMismatch
	}
	return pluginExecutorCleanupReasonLaunch
}
