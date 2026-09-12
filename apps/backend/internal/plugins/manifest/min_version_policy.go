package manifest

import "fmt"

// MinimumMessagesCapabilityVersion is the first Host release that supports
// both Go Messages().List and the browser conversation facade safely.
const MinimumMessagesCapabilityVersion = "0.91.1"

// RequiresMessagesCapabilityMinimum reports whether a manifest opts into the
// messages read resource whose Host contract requires a minimum release.
func RequiresMessagesCapabilityMinimum(manifest *Manifest) bool {
	return manifest != nil && manifest.Capabilities.CanRead("messages")
}

func (m *Manifest) validateCapabilityMinimumVersions() []error {
	if !RequiresMessagesCapabilityMinimum(m) {
		return nil
	}
	minimum, valid := NormalizeReleaseVersion(m.MinKandevVersion)
	if !valid || CompareVersions(minimum, MinimumMessagesCapabilityVersion) < 0 {
		return []error{fmt.Errorf(
			"api_read resource %q requires min_kandev_version >= %s",
			"messages",
			MinimumMessagesCapabilityVersion,
		)}
	}
	return nil
}
