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
	return containsString(m.Capabilities.APIRead, resource)
}

// CanWrite reports whether the manifest declares write access to resource
// via Capabilities.APIWrite.
func (m *Manifest) CanWrite(resource string) bool {
	return containsString(m.Capabilities.APIWrite, resource)
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
