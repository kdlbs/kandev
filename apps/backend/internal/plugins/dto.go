package plugins

import "github.com/kandev/kandev/internal/plugins/store"

// InstallRequest is the JSON body of POST /api/plugins/install when
// installing from a URL: {"url": "https://.../plugin-1.0.0.tar.gz"}. The
// same endpoint also accepts a multipart/form-data upload with a "package"
// field instead of this body — see Controller.install.
type InstallRequest struct {
	URL string `json:"url"`
}

// InstallResponse is the body of a successful POST /api/plugins/install.
// Warning is set (and the plugin's status left as "error") when the
// package was installed but its initial spawn/handshake failed — the
// operator can inspect Plugin.Status and retry via POST /:id/enable.
type InstallResponse struct {
	Plugin  *store.Record `json:"plugin"`
	Warning string        `json:"warning,omitempty"`
}

// UpdateConfigRequest is the PATCH /api/plugins/:id body: the full
// operator-editable config, replacing whatever was previously stored (see
// store.Store.SetConfig).
type UpdateConfigRequest struct {
	Config map[string]any `json:"config"`
}

// UpdateSettingsRequest is the PUT /api/plugins/settings body: the
// instance-wide plugin preferences. AutoUpdateDefault is required (a bare
// bool, not a pointer) — the endpoint sets the default outright.
type UpdateSettingsRequest struct {
	AutoUpdateDefault bool `json:"auto_update_default"`
}

// SetAutoUpdateRequest is the PUT /api/plugins/:id/auto-update body: the
// per-plugin override. AutoUpdate is a tri-state — true/false force the plugin
// on/off, and a null (or omitted) value clears the override so the plugin
// inherits the instance-wide default again.
type SetAutoUpdateRequest struct {
	AutoUpdate *bool `json:"auto_update"`
}

// SyncResult is the body of a successful POST /api/plugins/sync (and the
// return value of Service.Sync / Service.bootScan): what the filesystem
// scan under the plugins directory found and did this run, per
// docs/specs/plugins/spec.md ("Filesystem sideloading & sync").
type SyncResult struct {
	// Added lists the plugin ids of directory sideloads
	// (<pluginsDir>/<id>/<version>/manifest.yaml found with no existing
	// {id}.yml record) registered this run. Always registered
	// StatusDisabled — sideloads are unverified and are never auto-spawned.
	Added []string `json:"added"`
	// Installed lists the plugin ids of dropped *.tar.gz packages installed
	// (and, on a healthy spawn, activated) via the normal verified install
	// pipeline (Service.Install) this run. Each source tarball is deleted
	// from the plugins directory once its install succeeds.
	Installed []string `json:"installed"`
	// Missing lists the plugin ids of existing records whose InstallPath no
	// longer exists on disk. Each is stopped (if running) and transitioned
	// to StatusError.
	Missing []string `json:"missing"`
	// Errors lists every per-item failure encountered this run: a rejected
	// dir-sideload candidate (invalid/mismatched manifest.yaml, or a
	// lower-priority version dir skipped in favor of the lexically greatest
	// when more than one is found for the same unregistered id), or a
	// dropped tarball that failed pkgtar's verify/validate pipeline and was
	// left in place.
	Errors []SyncError `json:"errors"`
}

// SyncError is one entry of SyncResult.Errors: the filesystem path the
// problem was found at, and a human-readable reason.
type SyncError struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
