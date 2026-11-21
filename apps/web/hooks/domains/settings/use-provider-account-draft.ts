"use client";

import { useState } from "react";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { serializeSettingsRevision } from "@/components/settings/settings-save-revision";
import type {
  AccountTierConfig,
  BillingTier,
  ProviderAccountLimit,
  TierLimits,
} from "@/lib/api/domains/provider-limits-api";
import type { useProviderLimits } from "./use-provider-limits";

function accountDraft(account: ProviderAccountLimit): AccountTierConfig {
  return { ...account.overrides, tiers: account.overrides.tiers ?? {} };
}

/**
 * Owner overrides of one provider account. The draft registers with the
 * page's shared settings save, which owns Save, Discard and the navigation
 * warning for it.
 */
export function useProviderAccountDraft(
  account: ProviderAccountLimit,
  provider: string,
  save: ReturnType<typeof useProviderLimits>["save"],
) {
  const [draft, setDraft] = useState<AccountTierConfig>(() => accountDraft(account));
  const [saved, setSaved] = useState<AccountTierConfig>(() => accountDraft(account));
  const [saving, setSaving] = useState(false);
  const revision = serializeSettingsRevision(draft);
  useSettingsSaveContributor({
    id: `provider-account-limits:${provider}:${account.account_id}`,
    revision,
    isDirty: revision !== serializeSettingsRevision(saved),
    save: async () => {
      const submitted = draft;
      setSaving(true);
      try {
        await save(provider, {
          account_id: account.account_id,
          account_limits: submitted,
          monthly_reset_at: null,
          monthly_reset_timezone: "",
          block_until: null,
        });
        setSaved(submitted);
      } finally {
        setSaving(false);
      }
    },
    discard: () => setDraft(saved),
  });
  const updateTier = (tier: BillingTier, patch: Partial<TierLimits>) => {
    setDraft((current) => ({
      ...current,
      tiers: { ...current.tiers, [tier]: { ...current.tiers[tier], ...patch } },
    }));
  };
  return { draft, setDraft, updateTier, saving };
}
