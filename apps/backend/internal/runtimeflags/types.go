package runtimeflags

import "context"

type RuntimeFlagKind string

const (
	KindFeature RuntimeFlagKind = "feature"
	KindDebug   RuntimeFlagKind = "debug"
)

type RuntimeFlagSource string

const (
	SourceEnv      RuntimeFlagSource = "env"
	SourceOverride RuntimeFlagSource = "override"
	SourceProfile  RuntimeFlagSource = "profile"
	SourceDefault  RuntimeFlagSource = "default"
)

type RuntimeFlagStability string

const (
	StabilityStable       RuntimeFlagStability = "stable"
	StabilityBeta         RuntimeFlagStability = "beta"
	StabilityExperimental RuntimeFlagStability = "experimental"
)

type RuntimeFlagRiskLevel string

const (
	RiskLow    RuntimeFlagRiskLevel = "low"
	RiskMedium RuntimeFlagRiskLevel = "medium"
	RiskHigh   RuntimeFlagRiskLevel = "high"
)

// RuntimeFlagAvailabilityProbe reports whether a flag can be enabled on the
// current host and, when it cannot, a stable machine-readable reason code the
// frontend translates. Nil means always available.
type RuntimeFlagAvailabilityProbe func() (available bool, reasonCode string)

type RuntimeFlagDefinition struct {
	Key             string               `json:"key"`
	EnvVar          string               `json:"env_var"`
	Kind            RuntimeFlagKind      `json:"kind"`
	Label           string               `json:"label"`
	Description     string               `json:"description"`
	Stability       RuntimeFlagStability `json:"stability"`
	RiskLevel       RuntimeFlagRiskLevel `json:"risk_level"`
	RiskDescription string               `json:"risk_description"`
	RestartRequired bool                 `json:"restart_required"`
	Mutable         bool                 `json:"mutable"`
	ImpliedEnvVars  []string             `json:"-"`
	// Available is an optional host-availability probe. Nil (every existing
	// registration) means always available, which keeps this additive: no
	// other flag's behavior, serialization, or admin-override path changes.
	Available RuntimeFlagAvailabilityProbe `json:"-"`
}

// RuntimeFlagAvailability reports why a flag cannot be enabled on this host.
// Present in RuntimeFlagState only when the flag is unavailable.
type RuntimeFlagAvailability struct {
	Available  bool   `json:"available"`
	ReasonCode string `json:"reason_code,omitempty"`
}

type RuntimeFlagState struct {
	Key                    string                   `json:"key"`
	Kind                   RuntimeFlagKind          `json:"kind"`
	Label                  string                   `json:"label"`
	Description            string                   `json:"description"`
	Stability              RuntimeFlagStability     `json:"stability"`
	RiskLevel              RuntimeFlagRiskLevel     `json:"risk_level"`
	RiskDescription        string                   `json:"risk_description"`
	EffectiveValue         bool                     `json:"effective_value"`
	DefaultValue           bool                     `json:"default_value"`
	OverrideValue          *bool                    `json:"override_value"`
	Source                 RuntimeFlagSource        `json:"source"`
	EnvVar                 string                   `json:"env_var"`
	EnvLocked              bool                     `json:"env_locked"`
	RestartRequired        bool                     `json:"restart_required"`
	RequiresRestartToApply bool                     `json:"requires_restart_to_apply"`
	Mutable                bool                     `json:"mutable"`
	Availability           *RuntimeFlagAvailability `json:"availability,omitempty"`
}

type Store interface {
	ListOverrides(ctx context.Context) (map[string]bool, error)
	SetOverride(ctx context.Context, key string, value bool) error
	DeleteOverride(ctx context.Context, key string) error
}
