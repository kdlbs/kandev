package agents

import "time"

// RuntimeComponentRole identifies an observed component within an agent runtime.
type RuntimeComponentRole string

const (
	RuntimeComponentBridge   RuntimeComponentRole = "bridge"   // ACP bridge or primary runtime.
	RuntimeComponentProvider RuntimeComponentRole = "provider" // Runtime behind an ACP bridge.
)

// RuntimeComponentSource identifies who supplies a runtime component.
type RuntimeComponentSource string

const (
	RuntimeComponentManaged  RuntimeComponentSource = "managed"  // Selected from Kandev's managed runtime versions.
	RuntimeComponentBundled  RuntimeComponentSource = "bundled"  // Installed inside the managed bridge tree.
	RuntimeComponentExternal RuntimeComponentSource = "external" // Supplied by a separate executable.
	RuntimeComponentUnknown  RuntimeComponentSource = "unknown"  // Source could not be verified.
)

// RuntimeComponentOwner identifies who maintains a runtime component.
type RuntimeComponentOwner string

const (
	RuntimeComponentOwnerKandev   RuntimeComponentOwner = "kandev"   // Kandev manages the component.
	RuntimeComponentOwnerExternal RuntimeComponentOwner = "external" // The operator maintains the component.
	RuntimeComponentOwnerUnknown  RuntimeComponentOwner = "unknown"  // Ownership could not be verified.
)

// RuntimeComponentDescriptor contains trusted registration metadata used by a
// read-only profile discovery probe. ExternalVersionEnv is interpreted only
// for the fixed Codex --version recipe.
type RuntimeComponentDescriptor struct {
	Name               string                 `json:"name"`
	Package            string                 `json:"package,omitempty"`
	Source             RuntimeComponentSource `json:"source,omitempty"`
	Owner              RuntimeComponentOwner  `json:"owner,omitempty"`
	ExternalVersionEnv string                 `json:"external_version_env,omitempty"`
	GuidanceURL        string                 `json:"guidance_url,omitempty"`
}

// RuntimeObservationDescriptor is server-owned metadata. It is carried only
// from the backend registry to its agentctl utility probe.
type RuntimeObservationDescriptor struct {
	Bridge   RuntimeComponentDescriptor  `json:"bridge"`
	Provider *RuntimeComponentDescriptor `json:"provider,omitempty"`
}

// RuntimeObservationAgent optionally describes the provider runtime behind
// an ACP bridge. Agents without this capability expose bridge evidence only.
type RuntimeObservationAgent interface {
	// RuntimeProviderObservation returns trusted metadata for the provider behind an ACP bridge.
	RuntimeProviderObservation() RuntimeComponentDescriptor
}

// RuntimeInfo is an ephemeral, host-scoped observation tied to one profile
// model snapshot. It is never persisted independently from that snapshot.
type RuntimeInfo struct {
	Scope      string             `json:"scope"`
	ObservedAt time.Time          `json:"observed_at"`
	Components []RuntimeComponent `json:"components"`
}

// RuntimeComponent is one bounded, trusted runtime observation.
type RuntimeComponent struct {
	Role             RuntimeComponentRole   `json:"role"`
	Name             string                 `json:"name"`
	Package          string                 `json:"package,omitempty"`
	Source           RuntimeComponentSource `json:"source"`
	Owner            RuntimeComponentOwner  `json:"owner"`
	EffectiveVersion string                 `json:"effective_version,omitempty"`
	ObservedVersion  string                 `json:"observed_version,omitempty"`
	GuidanceURL      string                 `json:"guidance_url,omitempty"`
}
