"use client";

import type { Dispatch, SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type {
  AccountTierConfig,
  BillingTier,
  ProviderAccountLimit,
} from "@/lib/api/domains/provider-limits-api";
import { BILLING_TIERS, TIER_LABELS } from "@/lib/settings/provider-tier-limits";
import { settingsControlClassName } from "./settings-control";

type Props = {
  account: ProviderAccountLimit;
  provider: string;
  draft: AccountTierConfig;
  setDraft: Dispatch<SetStateAction<AccountTierConfig>>;
  disabled: boolean;
};

export function ProviderAccountOptions({ account, provider, draft, setDraft, disabled }: Props) {
  const { t } = useTranslation();
  if (provider === "opencode-go")
    return (
      <div className="grid gap-1">
        <Label htmlFor={`account-plan-${account.account_id}`}>{t("agents:providerTierPlan")}</Label>
        <Input
          disabled={disabled}
          id={`account-plan-${account.account_id}`}
          className={settingsControlClassName()}
          value={draft.subscription_plan ?? ""}
          placeholder={t("agents:providerTierPlanHint")}
          onChange={(event) =>
            setDraft((current) => ({
              ...current,
              subscription_plan: event.target.value || undefined,
            }))
          }
        />
      </div>
    );
  if (provider !== "llmgateway") return null;
  let creditStatus = "automatic";
  if (draft.purchased_credits !== undefined)
    creditStatus = draft.purchased_credits ? "purchased" : "zero";
  return (
    <div className="grid gap-1">
      <Label htmlFor={`account-credits-${account.account_id}`}>
        {t("agents:providerTierCreditStatus")}
      </Label>
      <Select
        disabled={disabled}
        value={creditStatus}
        onValueChange={(value) =>
          setDraft((current) => ({
            ...current,
            purchased_credits: value === "automatic" ? undefined : value === "purchased",
          }))
        }
      >
        <SelectTrigger
          id={`account-credits-${account.account_id}`}
          className={settingsControlClassName()}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="automatic">{t("agents:providerTierCreditUnknown")}</SelectItem>
          <SelectItem value="purchased">{t("agents:providerTierCreditPurchased")}</SelectItem>
          <SelectItem value="zero">{t("agents:providerTierCreditZero")}</SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

export function ProviderProfileTiers({
  account,
  draft,
  setDraft,
  disabled,
}: Omit<Props, "provider">) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-2">
      {account.profiles.map((profile) => (
        <div key={profile.id} className="grid gap-1">
          <Label htmlFor={`profile-tier-${profile.id}`}>{profile.name || profile.id}</Label>
          <Select
            value={draft.profile_tiers?.[profile.id] ?? "automatic"}
            disabled={disabled}
            onValueChange={(value) =>
              setDraft((current) => {
                const profiles = { ...current.profile_tiers };
                if (value === "automatic") delete profiles[profile.id];
                else profiles[profile.id] = value as BillingTier;
                return { ...current, profile_tiers: profiles };
              })
            }
          >
            <SelectTrigger id={`profile-tier-${profile.id}`} className={settingsControlClassName()}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="automatic">
                {t("agents:providerTierAutomatic", { tier: t(TIER_LABELS[profile.tier]) })}
              </SelectItem>
              {BILLING_TIERS.map((tier) => (
                <SelectItem key={tier} value={tier}>
                  {t(TIER_LABELS[tier])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ))}
    </div>
  );
}
