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
}

type RuntimeFlagState struct {
	Key                    string               `json:"key"`
	Kind                   RuntimeFlagKind      `json:"kind"`
	Label                  string               `json:"label"`
	Description            string               `json:"description"`
	Stability              RuntimeFlagStability `json:"stability"`
	RiskLevel              RuntimeFlagRiskLevel `json:"risk_level"`
	RiskDescription        string               `json:"risk_description"`
	EffectiveValue         bool                 `json:"effective_value"`
	DefaultValue           bool                 `json:"default_value"`
	OverrideValue          *bool                `json:"override_value"`
	Source                 RuntimeFlagSource    `json:"source"`
	EnvVar                 string               `json:"env_var"`
	EnvLocked              bool                 `json:"env_locked"`
	RestartRequired        bool                 `json:"restart_required"`
	RequiresRestartToApply bool                 `json:"requires_restart_to_apply"`
	Mutable                bool                 `json:"mutable"`
}

type Store interface {
	ListOverrides(ctx context.Context) (map[string]bool, error)
	SetOverride(ctx context.Context, key string, value bool) error
	DeleteOverride(ctx context.Context, key string) error
}
