// Package store persists installed plugin records to the filesystem, as
// described in docs/plans/plugins/GRPC-CONTRACT.md §6 ("Package format").
//
// Managed plugins (runtime.type: binary) are gRPC subprocesses spawned by
// kandev — there is no credential machinery here anymore (no api_key, no
// webhook_secret, no HMAC). Auth is the spawn relationship itself plus the
// go-plugin handshake and AutoMTLS. A Record instead tracks where the
// package was extracted to on disk (InstallPath), which version is
// installed, whether the package was signed, and best-effort restart
// bookkeeping for the runtime manager's supervision loop.
package store

import (
	"errors"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
)

// Status values for a plugin registration, per the state machine in
// docs/specs/plugins/spec.md ("State machine").
const (
	StatusRegistered  = "registered"
	StatusActive      = "active"
	StatusError       = "error"
	StatusDisabled    = "disabled"
	StatusUninstalled = "uninstalled"
)

// ErrNotFound is returned by Get, Delete, GetConfig, and SetConfig when no
// record exists for the id.
var ErrNotFound = errors.New("plugin not found")

// Record is a stored plugin installation: the manifest extracted from the
// package, plus the runtime fields kandev manages. Manifest.Version is the
// authoritative installed version (accessible as Record.Version via
// embedding); InstallPath is where pkgtar.Install extracted the package
// (destRoot/<id>/<version>).
type Record struct {
	manifest.Manifest `yaml:",inline"`

	Status       string    `yaml:"status" json:"status"`
	InstallPath  string    `yaml:"install_path" json:"install_path"`
	Signed       bool      `yaml:"signed" json:"signed"`
	InstalledAt  time.Time `yaml:"installed_at" json:"installed_at"`
	RestartCount int       `yaml:"restart_count" json:"restart_count"`
}

// Store is the persistence interface for plugin installations and their
// operator-editable config.
type Store interface {
	List() ([]*Record, error)
	Get(id string) (*Record, error)
	Save(record *Record) error
	Delete(id string) error
	GetConfig(id string) (map[string]any, error)
	SetConfig(id string, config map[string]any) error
}

// _ compile-time asserts that FSStore (fs_store.go) implements Store.
var _ Store = (*FSStore)(nil)
