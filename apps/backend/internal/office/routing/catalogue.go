package routing

import "github.com/kandev/kandev/internal/agent/registry"

// v1AllowList is the static set of provider IDs eligible for routing in
// v1. It is filtered against the live registry so a workspace cannot
// configure a provider whose binary is missing on this host.
//
// The canonical list lives in the registry package as
// registry.RoutableProviderIDs so the E2E multi-mock registration path
// (KANDEV_MOCK_PROVIDERS) and this catalogue cannot drift.
var v1AllowList = buildV1AllowList()

func buildV1AllowList() []ProviderID {
	out := make([]ProviderID, 0, len(registry.RoutableProviderIDs))
	for _, id := range registry.RoutableProviderIDs {
		out = append(out, ProviderID(id))
	}
	return out
}

// KnownProviders returns the v1 allow-list intersected with the
// registry's enabled agents. Callers (HTTP validators, settings UI,
// resolver) should use this as the single source of truth for which
// providers can appear in a routing config.
//
// When reg is nil the function returns the static allow-list — useful
// for unit tests that exercise the validators without standing up a
// real registry.
func KnownProviders(reg *registry.Registry) []ProviderID {
	if reg == nil {
		out := make([]ProviderID, len(v1AllowList))
		copy(out, v1AllowList)
		return out
	}
	out := make([]ProviderID, 0, len(v1AllowList))
	for _, p := range v1AllowList {
		if reg.Exists(string(p)) {
			out = append(out, p)
		}
	}
	return out
}
