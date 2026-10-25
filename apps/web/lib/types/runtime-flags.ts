export type RuntimeFlagKind = "feature" | "debug";
export type RuntimeFlagSource = "env" | "override" | "profile" | "default";
export type RuntimeFlagStability = "stable" | "beta" | "experimental";
export type RuntimeFlagRiskLevel = "low" | "medium" | "high";

// reason_code is a stable, machine-readable identifier the frontend
// translates (unlike RestartCapability.reason, which is backend-authored
// freeform text rendered verbatim) -- see unavailableReasonMessage in
// feature-toggle-card.tsx.
export interface RuntimeFlagAvailability {
  available: boolean;
  reason_code?: string;
}

export interface RuntimeFlagState {
  key: string;
  kind: RuntimeFlagKind;
  label: string;
  description: string;
  stability: RuntimeFlagStability;
  risk_level: RuntimeFlagRiskLevel;
  risk_description: string;
  effective_value: boolean;
  default_value: boolean;
  override_value: boolean | null;
  source: RuntimeFlagSource;
  env_var: string;
  env_locked: boolean;
  restart_required: boolean;
  requires_restart_to_apply: boolean;
  mutable: boolean;
  // Present only when the flag cannot be enabled on this host (omitempty on
  // the wire). Absent/null means available.
  availability?: RuntimeFlagAvailability | null;
}

export interface RuntimeFlagsResponse {
  flags: RuntimeFlagState[];
}
