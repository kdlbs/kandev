// Package plugins implements the core plugin Service: package install/
// uninstall (internal/plugins/pkgtar), the in-memory Registry loaded from
// the filesystem store (internal/plugins/store), the lifecycle state
// machine, spawning/supervision via internal/plugins/runtime, the Host RPC
// implementation (host.go) plugins call back into, and event delivery
// wiring (internal/plugins/delivery).
package plugins

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// Status is a plugin's lifecycle state, per the state machine in
// docs/specs/plugins/spec.md ("State machine"):
//
//	registered -> active -> disabled -> uninstalled
//	                 |          |
//	                 +-> error -+
//
// Status is a type alias (not a distinct type) for store's Record.Status
// field, and the constants below are direct aliases of the string values
// store already defines on Record — store.go is the single source of truth
// for the status vocabulary; this package never redefines it.
type Status = string

// Status values, aliased from internal/plugins/store so callers can use
// either plugins.StatusActive or store.StatusActive interchangeably.
const (
	StatusRegistered  = store.StatusRegistered
	StatusActive      = store.StatusActive
	StatusError       = store.StatusError
	StatusDisabled    = store.StatusDisabled
	StatusUninstalled = store.StatusUninstalled
)

// ErrInvalidTransition is returned by Service.SetStatus (and the Enable /
// Disable / SetStatus family) when the requested status change is not a
// legal single-hop edge in the state machine.
type ErrInvalidTransition struct {
	ID   string
	From Status
	To   Status
}

func (e *ErrInvalidTransition) Error() string {
	return fmt.Sprintf("plugin %q: invalid status transition %s -> %s", e.ID, e.From, e.To)
}

// allowedTransitions enumerates the legal single-hop edges of the state
// machine. Notably absent: any edge into StatusUninstalled. Uninstalling is
// not a status transition in this implementation — Service.Uninstall
// performs a hard delete (stop the runtime process, remove the extracted
// package, delete the record) instead of persisting status:
// "uninstalled".
var allowedTransitions = map[Status][]Status{
	StatusRegistered: {StatusActive, StatusError},
	StatusActive:     {StatusDisabled, StatusError},
	StatusError:      {StatusActive, StatusDisabled},
	StatusDisabled:   {StatusActive},
}

// canTransition reports whether to is a legal single-hop transition from
// from. Same-status "transitions" are rejected here; Enable/Disable treat
// those as idempotent no-ops before ever calling SetStatus.
func canTransition(from, to Status) bool {
	for _, allowed := range allowedTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// PluginRuntime is the subset of *runtime.Manager's surface Service
// depends on, defined here (consumer side) so Service stays testable
// against a fake without spawning real subprocesses. *runtime.Manager
// satisfies this structurally.
type PluginRuntime interface {
	// Start spawns and begins supervising rec's process. hostFactory is
	// called by the runtime manager (possibly more than once, across
	// restarts) to build the Host implementation bound to a given plugin
	// id at spawn time.
	Start(ctx context.Context, rec *store.Record, hostFactory func(pluginID string) pluginsdk.Host) error
	// Stop kills and stops supervising id's process. A no-op if not running.
	Stop(id string)
	// Get returns the live RemotePlugin for id, and whether one exists.
	Get(id string) (*pluginsdk.RemotePlugin, bool)
	// Ping issues an on-demand health check against id's current process.
	Ping(id string) error
	// Running reports whether id currently has a live process.
	Running(id string) bool
	// RestartCount returns how many times id's process has been
	// automatically restarted since it was started.
	RestartCount(id string) int
	// StopAll stops every currently-running plugin. Used for graceful
	// backend shutdown.
	StopAll()
}

// SecretRevealer is the minimal surface Service needs to resolve a secret
// reference to its cleartext value, for the Host.RevealSecret RPC.
// Satisfied by *internal/integrations/secretadapter.Adapter in production;
// tests use a fake.
type SecretRevealer interface {
	Reveal(ctx context.Context, ref string) (string, error)
}

// Deliverer is the minimal surface the event-delivery subsystem
// (internal/plugins/delivery) exposes back to Service. Defined here
// (consumer side) so internal/plugins/delivery does not need to import this
// package — its Deliverer type satisfies this interface structurally.
//
// Service.SetDeliverer wires an implementation in post-construction (see
// the "Extension points" doc comment on Service), mirroring the
// SetTaskDeleter / SetRepositoryLookup pattern used in internal/jira to
// avoid import cycles between sibling packages.
type Deliverer interface {
	// Refresh re-subscribes to the event bus based on the current registry
	// state. Service calls this after every mutation that can change which
	// plugins/events should receive deliveries: Install, Uninstall, Enable,
	// Disable, and any other successful SetStatus transition.
	Refresh()

	// Flush delivers any events buffered for pluginID while it was in the
	// error state, in order. Service calls this itself after an
	// error -> active recovery transition driven by the runtime manager's
	// OnStatusChange callback.
	Flush(pluginID string)
}
