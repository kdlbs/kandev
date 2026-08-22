// --- Provider routing types ---
//
// Split out of `types.ts` to keep that file under the repo's 600-line cap
// (see apps/web/AGENTS.md "Code-quality limits"). Re-exported from
// `types.ts` so existing `@/lib/state/slices/office/types` imports keep
// working unchanged.

import type { AgentRole } from "@/lib/types/agent-profile";

export type Tier = "frontier" | "balanced" | "economy";

export type RoutingErrorCode =
  | "auth_required"
  | "missing_credentials"
  | "subscription_required"
  | "quota_limited"
  | "rate_limited"
  | ("network_unavailable" | "provider_unavailable" | "provider_overloaded" | "model_capacity")
  | "model_unavailable"
  | "provider_not_configured"
  | "unknown_provider_error"
  | "agent_runtime_error"
  | "task_error"
  | "repo_error"
  | "permission_denied_by_user"
  | "agent_transport_lost";

export type TierMap = {
  frontier?: string;
  balanced?: string;
  economy?: string;
};

export type ProviderProfile = {
  tier_map: TierMap;
  execution_profile_ids?: Partial<Record<Tier, string>>;
  /** @deprecated Accepted only while reading pre-migration routing payloads. */
  tier_profile_ids?: Partial<Record<Tier, string>>;
  mode?: string;
  flags?: string[];
  env?: Record<string, string>;
};

export type ExecutionProfileSummary = {
  id: string;
  name: string;
  provider_id: string;
  model: string;
  mode?: string;
  workspace_id?: string;
};

// Wake reasons the workspace can map onto specific tiers. v1 keeps the
// surface narrow — only the three reasons that historically used the
// cheap-profile shortcut. Adding more requires both a backend constant
// and a UI copy update so the user gets context for each row.
export type WakeReason = "heartbeat" | "routine_trigger" | "budget_alert";

export type TierPerReason = Partial<Record<WakeReason, Tier>>;

// Per-role tier policy: workspace-level default for every agent of a given
// role, overridable per-agent and shadowed by tier_per_reason. See
// docs/specs/office-agent-tier-routing/spec.md.
export type RoleTierMap = Partial<Record<AgentRole, Tier>>;

// The four precedence levels a resolved tier can come from, in
// highest-to-lowest priority order: a matching wake-reason policy, a
// per-agent override, the agent's role entry, or the workspace default.
export type TierSource = "wake_reason" | "override" | "role" | "workspace";

export type WorkspaceRouting = {
  enabled: boolean;
  provider_order: string[];
  default_tier: Tier;
  provider_profiles: Record<string, ProviderProfile>;
  tier_per_reason?: TierPerReason;
  role_tiers?: RoleTierMap;
};

export type AgentRoutingOverrides = {
  provider_order_source?: "inherit" | "override" | "";
  provider_order?: string[];
  tier_source?: "inherit" | "override" | "";
  tier?: Tier | "";
  tier_per_reason_source?: "inherit" | "override" | "";
  tier_per_reason?: TierPerReason;
};

export type ProviderHealthState = "healthy" | "short_retry" | "degraded" | "user_action_required";
export type ProviderHealthScope = "provider" | "model" | "tier";

export type ProviderHealth = {
  workspace_id?: string;
  provider_id: string;
  scope: ProviderHealthScope;
  scope_value: string;
  state: ProviderHealthState;
  error_code?: RoutingErrorCode;
  retry_at?: string;
  backoff_step: number;
  last_failure?: string;
  last_success?: string;
  raw_excerpt?: string;
};

export type RouteAttemptOutcome =
  | "launched"
  | "retry_scheduled"
  | "failed_provider_unavailable"
  | "failed_other"
  | "skipped_degraded"
  | "skipped_user_action"
  | "skipped_missing_mapping"
  | "skipped_max_attempts";

export type RouteAttempt = {
  seq: number;
  execution_profile_id?: string;
  provider_id: string;
  model?: string;
  tier: Tier | "";
  // Empty for attempts recorded before this column existed, or that never
  // resolved a tier (e.g. a max-attempts-exceeded row).
  tier_source?: TierSource | "";
  outcome: RouteAttemptOutcome;
  error_code?: RoutingErrorCode;
  error_confidence?: "high" | "medium" | "low";
  adapter_phase?: string;
  classifier_rule?: string;
  exit_code?: number;
  raw_excerpt?: string;
  reset_hint?: string;
  started_at: string;
  finished_at?: string;
};

export type ProviderModelPair = {
  execution_profile_id?: string;
  provider_id: string;
  model: string;
  tier: Tier | "";
};

export type AgentRoutePreview = {
  agent_id: string;
  agent_name: string;
  tier_source: TierSource;
  effective_tier: Tier;
  // primary_* reflects configured intent — first entry in the
  // effective provider order, even when that provider is currently
  // skipped (degraded / missing mapping).
  primary_provider_id?: string;
  primary_execution_profile_id?: string;
  primary_model?: string;
  // current_* reflects the candidate the next launch would actually
  // pick; equal to primary when not degraded. Empty when every
  // candidate is skipped.
  current_provider_id?: string;
  current_execution_profile_id?: string;
  current_model?: string;
  fallback_chain: ProviderModelPair[];
  missing: string[];
  degraded: boolean;
};

export type AgentRouteData = {
  preview: AgentRoutePreview;
  /**
   * Raw routing override blob persisted on the agent's settings. The
   * agent routing UI hydrates from this so toggles + tier override +
   * provider-order override reflect persisted values on first paint.
   */
  overrides: AgentRoutingOverrides;
  last_failure_code?: RoutingErrorCode;
  last_failure_run?: string;
};

export type RoutingState = {
  byWorkspace: Record<string, WorkspaceRouting | undefined>;
  knownProviders: string[];
  preview: { byWorkspace: Record<string, AgentRoutePreview[] | undefined> };
};

export type ProviderHealthSliceState = {
  byWorkspace: Record<string, ProviderHealth[]>;
};

export type RunAttemptsState = {
  byRunId: Record<string, RouteAttempt[]>;
};

export type AgentRoutingSliceState = {
  byAgentId: Record<string, AgentRouteData | undefined>;
};
