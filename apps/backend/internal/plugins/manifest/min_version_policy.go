package manifest

import "fmt"

// MinimumMessagesCapabilityVersion is the first Host release that supports
// both Go Messages().List and the browser conversation facade safely.
const MinimumMessagesCapabilityVersion = "0.91.1"

// MinimumInstanceReadContextsVersion is the first Host release that supports
// the coordinator and external agent-tool contexts.
const MinimumInstanceReadContextsVersion = "0.99.0"

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
	return errs
}
