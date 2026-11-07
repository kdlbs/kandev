"use client";

import { useRef, useState } from "react";
import type {
  AccountTierConfig,
  BillingTier,
  ProviderAccountLimit,
  TierLimits,
} from "@/lib/api/domains/provider-limits-api";
import type { useProviderLimits } from "./use-provider-limits";

export function useProviderAccountDraft(
  account: ProviderAccountLimit,
  provider: string,
  save: ReturnType<typeof useProviderLimits>["save"],
) {
  const [draft, setDraft] = useState<AccountTierConfig>(() => ({
    ...account.overrides,
    tiers: account.overrides.tiers ?? {},
  }));
  const [status, setStatus] = useState<"idle" | "saving" | "saved" | "failed">("idle");
  const pending = useRef(false);
  const updateTier = (tier: BillingTier, patch: Partial<TierLimits>) => {
    setDraft((current) => ({
      ...current,
      tiers: { ...current.tiers, [tier]: { ...current.tiers[tier], ...patch } },
    }));
    setStatus("idle");
  };
  const submit = async () => {
    if (pending.current) return;
    pending.current = true;
    setStatus("saving");
    try {
      await save(provider, {
        account_id: account.account_id,
        account_limits: draft,
        monthly_reset_at: null,
        monthly_reset_timezone: "",
        block_until: null,
      });
      setStatus("saved");
    } catch {
      setStatus("failed");
    } finally {
      pending.current = false;
    }
  };
  const editDraft: typeof setDraft = (next) => {
    setDraft(next);
    setStatus("idle");
  };
  return { draft, setDraft: editDraft, updateTier, submit, status };
}
