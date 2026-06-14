export type RuntimeFlagKind = "feature" | "debug";
export type RuntimeFlagSource = "env" | "override" | "profile" | "default";
export type RuntimeFlagStability = "stable" | "beta" | "experimental";
export type RuntimeFlagRiskLevel = "low" | "medium" | "high";

export interface RuntimeFlagState {
  key: string;
  kind: RuntimeFlagKind;
  label: string;
  description: string;
  stability: RuntimeFlagStability;
  risk_level: RuntimeFlagRiskLevel;
  risk_description?: string;
  effective_value: boolean;
  default_value: boolean;
  override_value?: boolean | null;
  source: RuntimeFlagSource;
  env_var: string;
  env_locked: boolean;
  restart_required: boolean;
  requires_restart_to_apply: boolean;
  mutable: boolean;
}

export interface RuntimeFlagsResponse {
  flags: RuntimeFlagState[];
}
