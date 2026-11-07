"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useProviderAccountDraft } from "@/hooks/domains/settings/use-provider-account-draft";
import type { useProviderLimits } from "@/hooks/domains/settings/use-provider-limits";
import type { BillingTier, ProviderAccountLimit } from "@/lib/api/domains/provider-limits-api";
import { settingsActionClassName } from "./settings-control";
import { ProviderTierEditor } from "./provider-tier-editor";
import {
  BILLING_TIERS,
  TIER_LABELS,
  freeTierModels,
  getProviderTierState,
} from "@/lib/settings/provider-tier-limits";
import { ProviderAccountOptions, ProviderProfileTiers } from "./provider-account-options";

export function ProviderAccountLimits({
  account,
  provider,
  save,
}: {
  account: ProviderAccountLimit;
  provider: string;
  save: ReturnType<typeof useProviderLimits>["save"];
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [selected, setSelected] = useState<BillingTier>("free");
  const { draft, setDraft, updateTier, submit, status } = useProviderAccountDraft(
    account,
    provider,
    save,
  );
  const visibleTiers = isMobile ? [selected] : BILLING_TIERS;
  const freeModels = freeTierModels(account);
  const statusLabels = {
    idle: "",
    saving: "agents:providerTierSaving",
    saved: "agents:providerLimitsSaved",
    failed: "agents:providerLimitsSaveFailed",
  };
  return (
    <section className="grid min-w-0 gap-3 rounded-md border p-3" aria-label={account.account_id}>
      <h3 className="break-all font-medium">{account.account_id}</h3>
      <p className="text-xs text-muted-foreground">{t("agents:providerTierMeasuredHint")}</p>
      <ProviderAccountOptions
        account={account}
        provider={provider}
        draft={draft}
        setDraft={setDraft}
        disabled={status === "saving"}
      />
      {isMobile ? (
        <div role="tablist" className="grid grid-cols-3 gap-1">
          {BILLING_TIERS.map((tier) => (
            <Button
              key={tier}
              role="tab"
              aria-selected={selected === tier}
              className={settingsActionClassName()}
              variant={selected === tier ? "secondary" : "outline"}
              onClick={() => setSelected(tier)}
            >
              {t(TIER_LABELS[tier])}
            </Button>
          ))}
        </div>
      ) : null}
      <div className={isMobile ? "min-w-0" : "grid min-w-0 grid-cols-3 gap-3"}>
        {visibleTiers.map((tier) => (
          <ProviderTierEditor
            key={tier}
            accountId={account.account_id}
            tier={tier}
            models={freeModels}
            state={getProviderTierState(account, tier)}
            override={draft.tiers[tier] ?? {}}
            onChange={(patch) => updateTier(tier, patch)}
            disabled={status === "saving"}
          />
        ))}
      </div>
      <ProviderProfileTiers
        account={account}
        draft={draft}
        setDraft={setDraft}
        disabled={status === "saving"}
      />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          data-testid={`account-save-${account.account_id}`}
          className={settingsActionClassName()}
          disabled={status === "saving"}
          onClick={() => void submit()}
        >
          {t("agents:providerLimitsSave")}
        </Button>
        <Button
          variant="ghost"
          className={settingsActionClassName()}
          disabled={status === "saving"}
          onClick={() => setDraft({ tiers: {} })}
        >
          {t("agents:providerTierResetOverrides")}
        </Button>
        {status !== "idle" ? (
          <span role={status === "failed" ? "alert" : "status"} className="text-xs">
            {t(statusLabels[status])}
          </span>
        ) : null}
      </div>
    </section>
  );
}
