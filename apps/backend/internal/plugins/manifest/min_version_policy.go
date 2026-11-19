package manifest

import "fmt"

// MinimumMessagesCapabilityVersion is the first Host release that supports
// both Go Messages().List and the browser conversation facade safely.
const MinimumMessagesCapabilityVersion = "0.91.1"

// MinimumInstanceReadContextsVersion is the first Host release that supports
// the coordinator and external agent-tool contexts.
const MinimumInstanceReadContextsVersion = "0.99.0"

// MinimumWorkspaceScopeVersion is the first Host release that supports
// workspace-scoped agent tools.
const MinimumWorkspaceScopeVersion = "0.100.0"

// MinimumSidebarGroupingVersion reserves the unreleased Host compatibility floor.
// Stable 0.102.0 predates complete server-owned sidebar classification.
const MinimumSidebarGroupingVersion = "0.103.0"

// RequiresMessagesCapabilityMinimum reports whether a manifest opts into the
// messages read resource whose Host contract requires a minimum release.
func RequiresMessagesCapabilityMinimum(manifest *Manifest) bool {
	return manifest != nil && manifest.Capabilities.CanRead("messages")
}

// RequiresInstanceReadContextMinimum reports whether a manifest declares a
// surface that first appeared in the instance-read context release.
func RequiresInstanceReadContextMinimum(manifest *Manifest) bool {
	if manifest == nil {
		return false
	}
	for _, tool := range manifest.AgentTools {
		for _, surface := range tool.Surfaces {
			if surface == AgentToolSurfaceCoordinator || surface == AgentToolSurfaceExternal {
				return true
			}
		}
	}
	return false
}

// RequiresWorkspaceScopeMinimum reports whether a manifest declares a
// workspace-scoped tool that first appeared in the workspace scope release.
func RequiresWorkspaceScopeMinimum(manifest *Manifest) bool {
	if manifest == nil {
		return false
	}
	for _, tool := range manifest.AgentTools {
		if tool.NormalizedInvocationScope() == AgentToolInvocationScopeWorkspace {
			return true
		}
	}
	return false
}

func (m *Manifest) validateCapabilityMinimumVersions() []error {
	var errs []error
	if RequiresMessagesCapabilityMinimum(m) {
		minimum, valid := NormalizeReleaseVersion(m.MinKandevVersion)
		if !valid || CompareVersions(minimum, MinimumMessagesCapabilityVersion) < 0 {
			errs = append(errs, fmt.Errorf(
				"api_read resource %q requires min_kandev_version >= %s",
				"messages",
				MinimumMessagesCapabilityVersion,
			))
		}
	}
	if RequiresInstanceReadContextMinimum(m) {
		minimum, valid := NormalizeReleaseVersion(m.MinKandevVersion)
		if !valid || CompareVersions(minimum, MinimumInstanceReadContextsVersion) < 0 {
			errs = append(errs, fmt.Errorf(
				"agent_tools surfaces coordinator/external require min_kandev_version >= %s",
				MinimumInstanceReadContextsVersion,
			))
		}
	}
	if RequiresWorkspaceScopeMinimum(m) {
		minimum, valid := NormalizeReleaseVersion(m.MinKandevVersion)
		if !valid || CompareVersions(minimum, MinimumWorkspaceScopeVersion) < 0 {
			errs = append(errs, fmt.Errorf(
				"agent_tools invocation_scope: workspace requires min_kandev_version >= %s",
				MinimumWorkspaceScopeVersion,
			))
		}
	}
	if len(m.SidebarGroupings) > 0 {
		minimum, valid := NormalizeReleaseVersion(m.MinKandevVersion)
		if !valid || CompareVersions(minimum, MinimumSidebarGroupingVersion) < 0 {
			errs = append(errs, fmt.Errorf("sidebar_groupings requires min_kandev_version >= %s", MinimumSidebarGroupingVersion))
		}
	}
	return errs
}
