import { fetchJson, type ApiRequestOptions } from "@/lib/api/client";

/** Where the reset routing uses for a provider came from. */
export type ProviderResetSource = "usage" | "manual";

export type ProviderLimit = {
  accounts?: ProviderAccountLimit[];
  provider: string;
  profile_count: number;
  /** Usage limits of this provider pause one model at a time. */
  model_scoped: boolean;
  /** One occurrence of the operator-entered monthly reset. */
  monthly_reset_at?: string;
  monthly_reset_timezone?: string;
  /** The monthly reset routing uses, observed usage first. */
  next_monthly_reset?: string;
  next_monthly_reset_source?: ProviderResetSource;
  /** Every paid model of the provider is paused until this instant. */
  block_until?: string;
  observed_exhausted_until?: string;
  updated_at?: string;
};

export type ProviderLimitsResponse = {
  providers: ProviderLimit[];
};

export type UpdateProviderLimitRequest = {
  account_id?: string;
  account_limits?: AccountTierConfig;
  monthly_reset_at: string | null;
  monthly_reset_timezone: string;
  block_until: string | null;
};

export type BillingTier = "free" | "subscription" | "paid";
export type LimitBudget = {
  window_estimated?: boolean;
  window: "5h" | "day" | "week" | "month";
  unit: "usd" | "tokens" | "percent" | "requests" | "turns";
  value?: number;
  reset: "rolling" | "utc" | "provider" | "anchor";
  reset_at?: string;
  timezone?: string;
  scope?: string;
};
export type LimitProvenance = "published" | "measured" | "approximate" | "provider";
/** A daily request allowance for one model or a shared group of models. */
export type RequestScope = {
  id: string;
  kind?: "model" | "group";
  models?: string[];
  requests_per_day?: number;
  daily_reset?: string;
  provenance?: LimitProvenance;
  learned?: boolean;
  provisional?: boolean;
  learned_at?: string;
};
export type TierLimits = {
  scopes?: RequestScope[];
  provenance?: Record<string, LimitProvenance>;
  max_concurrent_turns?: number;
  requests_per_minute?: number;
  requests_per_day?: number;
  daily_reset?: string;
  requests_per_window?: number;
  request_window_seconds?: number;
  requests_per_model?: boolean;
  windows?: LimitBudget[];
  learned?: boolean;
};
export type AccountTierConfig = {
  subscription_plan?: string;
  tiers: Partial<Record<BillingTier, TierLimits>>;
  profile_tiers?: Record<string, BillingTier>;
  purchased_credits?: boolean;
};
export type ProviderAccountLimit = {
  account_id: string;
  account_kind: string;
  profiles: { id: string; name: string; tier: BillingTier; model?: string }[];
  overrides: AccountTierConfig;
  tiers: Record<
    BillingTier,
    {
      defaults: TierLimits;
      learned: TierLimits;
      override: TierLimits;
      effective: TierLimits;
      usage: {
        active_turns?: number;
        requests_minute?: number;
        requests_day?: number;
        turns_day?: number;
        tokens_day?: number;
        scopes?: Record<string, number>;
        estimated?: boolean;
      };
    }
  >;
};

export async function listProviderLimits(
  options?: ApiRequestOptions,
): Promise<ProviderLimitsResponse> {
  return fetchJson<ProviderLimitsResponse>("/api/v1/provider-limits", options);
}

export async function updateProviderLimit(
  provider: string,
  request: UpdateProviderLimitRequest,
  options?: ApiRequestOptions,
): Promise<ProviderLimit> {
  return fetchJson<ProviderLimit>(`/api/v1/provider-limits/${encodeURIComponent(provider)}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify(request), ...(options?.init ?? {}) },
  });
}
