package manifest

// runtimeTypeBinary is the only currently supported Runtime.Type value.
const runtimeTypeBinary = "binary"

// IsManaged reports whether the manifest declares a kandev-managed binary
// runtime (runtime.type: binary), as opposed to a legacy-remote manifest
// that registers an already-running service via base_url/endpoints.
func (m *Manifest) IsManaged() bool {
	return m.Runtime.Type == runtimeTypeBinary
}

// HasWebApps reports whether the manifest declares one or more isolated
// static web applications.
func (m *Manifest) HasWebApps() bool {
	return m != nil && len(m.UI.WebApps) > 0
}

// IsStaticWebAppOnly reports the package form that has no managed backend and
// contributes only isolated static web applications.
func (m *Manifest) IsStaticWebAppOnly() bool {
	return m != nil && m.HasWebApps() && !m.IsManaged() && m.BaseURL == "" && m.Endpoints == (Endpoints{})
}

// ExecutableFor returns the package-relative executable path declared for
// the given host platform (goos/goarch, e.g. "linux", "amd64"), and whether
// an entry exists. The returned path is relative to the extracted package
// root and already includes any platform-specific suffix (e.g. ".exe" on
// Windows); callers must not append anything to it.
func (m *Manifest) ExecutableFor(goos, goarch string) (string, bool) {
	path, ok := m.Runtime.Executables[goos+"-"+goarch]
	return path, ok
}
