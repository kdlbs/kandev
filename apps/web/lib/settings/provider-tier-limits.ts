import type {
  BillingTier,
  LimitBudget,
  ProviderAccountLimit,
  RequestScope,
  TierLimits,
} from "@/lib/api/domains/provider-limits-api";

export const PROVENANCE_LABELS = {
  published: "agents:providerLimitProvenancePublished",
  measured: "agents:providerLimitProvenanceMeasured",
  approximate: "agents:providerLimitProvenanceApproximate",
  provider: "agents:providerLimitProvenanceProvider",
};

/** Provider model IDs of the account's free-tier profiles, for new request scopes. */
export function freeTierModels(account: ProviderAccountLimit): string[] {
  const models = account.profiles.flatMap((profile) =>
    profile.tier === "free" && profile.model ? [profile.model] : [],
  );
  return [...new Set(models)].sort();
}

/** Saved scopes followed by owner scopes added in the unsaved draft. */
export function scopeRows(effective: TierLimits, override: TierLimits): RequestScope[] {
  const rows = [...(effective.scopes ?? [])];
  for (const scope of override.scopes ?? [])
    if (scope.kind && !rows.some((row) => row.id === scope.id)) rows.push(scope);
  return rows;
}

/**
 * Sets or clears the owner value of one scope. An existing owner entry keeps
 * its other fields (kind, models, daily reset); clearing drops the entry only
 * when the value was all it carried.
 */
export function updateScopeOverride(
  overrides: RequestScope[],
  scope: RequestScope,
  value: number | undefined,
): RequestScope[] {
  const previous = overrides.find((entry) => entry.id === scope.id);
  if (!previous) {
    return value === undefined
      ? overrides
      : [...overrides, { id: scope.id, requests_per_day: value }];
  }
  const next = { ...previous, requests_per_day: value };
  const keep = Object.entries(next).some(
    ([key, field]) => key !== "id" && key !== "requests_per_day" && field !== undefined,
  );
  if (value === undefined && !keep) return overrides.filter((entry) => entry.id !== scope.id);
  return overrides.map((entry) => (entry.id === scope.id ? next : entry));
}

/** An owner scope for the selected models: one model or a shared group. */
export function newOwnerScope(models: string[], value: number | undefined): RequestScope {
  const sorted = [...models].sort();
  const kind = sorted.length === 1 ? "model" : "group";
  return { id: `${kind}:${sorted.join("+")}`, kind, models: sorted, requests_per_day: value };
}

export const BILLING_TIERS: BillingTier[] = ["free", "subscription", "paid"];
export const TIER_LABELS = {
  free: "agents:providerTierFree",
  subscription: "agents:providerTierSubscription",
  paid: "agents:providerTierPaid",
};

export function getProviderTierState(account: ProviderAccountLimit, tier: BillingTier) {
  return (
    account.tiers[tier] ?? { defaults: {}, learned: {}, override: {}, effective: {}, usage: {} }
  );
}

export function budgetKey(budget: LimitBudget) {
  return `${budget.window}:${budget.scope ?? ""}`;
}

export function updateBudgetOverride(
  overrides: LimitBudget[],
  budget: LimitBudget,
  patch: Partial<LimitBudget>,
  clear = false,
) {
  const previous = overrides.find((entry) => budgetKey(entry) === budgetKey(budget));
  const windows = overrides.filter((entry) => budgetKey(entry) !== budgetKey(budget));
  if (!clear)
    windows.push({
      window: budget.window,
      scope: budget.scope,
      unit: budget.unit,
      reset: budget.reset,
      reset_at: budget.reset_at,
      timezone: budget.timezone,
      ...previous,
      ...patch,
    });
  return windows;
}
