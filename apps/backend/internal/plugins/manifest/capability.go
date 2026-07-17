package manifest

// HasEvent reports whether the manifest's declared event subscriptions
// (Capabilities.Events) cover the given concrete event name, including
// wildcard subscriptions such as "task.*".
func (m *Manifest) HasEvent(name string) bool {
	for _, pattern := range m.Capabilities.Events {
		if MatchSubject(pattern, name) {
			return true
		}
	}
	return false
}

// CanRead reports whether the manifest declares read access to resource via
// Capabilities.APIRead.
func (m *Manifest) CanRead(resource string) bool {
	return m.Capabilities.CanRead(resource)
}

// CanWrite reports whether the manifest declares write access to resource
// via Capabilities.APIWrite.
func (m *Manifest) CanWrite(resource string) bool {
	return m.Capabilities.CanWrite(resource)
}

// CanRead reports whether c declares read access to resource via APIRead
// (ADR 0042's api_read:<resource> capabilities, e.g. "tasks", "sessions").
// Exposed on Capabilities directly (not just Manifest) so callers that only
// hold a plugin's currently-registered Capabilities snapshot — such as
// internal/plugins.pluginHost, bound at spawn time — can gate without a full
// Manifest.
func (c Capabilities) CanRead(resource string) bool {
	return containsString(c.APIRead, resource)
}

// CanWrite reports whether c declares write access to resource via
// APIWrite. See CanRead's doc comment for why this also lives on
// Capabilities directly.
func (c Capabilities) CanWrite(resource string) bool {
	return containsString(c.APIWrite, resource)
}

// HasUIBundle reports whether the manifest declares a native UI bundle via
// UISection.Bundle.
func (m *Manifest) HasUIBundle() bool {
	return m.UI.Bundle != ""
}

// containsString reports whether target is present in values.
func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
