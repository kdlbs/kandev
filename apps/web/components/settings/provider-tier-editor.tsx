"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type {
  BillingTier,
  ProviderAccountLimit,
  TierLimits,
} from "@/lib/api/domains/provider-limits-api";
import { settingsControlClassName } from "./settings-control";
import { TIER_LABELS } from "@/lib/settings/provider-tier-limits";
import { ProviderTierBudgets } from "./provider-tier-budgets";
import { ProviderTierScopes } from "./provider-tier-scopes";
import { PROVENANCE_LABELS } from "@/lib/settings/provider-tier-limits";

type NumericField =
  | "max_concurrent_turns"
  | "requests_per_minute"
  | "requests_per_day"
  | "requests_per_window"
  | "request_window_seconds";
const BASE_FIELDS: { field: NumericField; label: string }[] = [
  { field: "max_concurrent_turns", label: "agents:providerTierConcurrency" },
  { field: "requests_per_minute", label: "agents:providerTierMinute" },
];
const FREE_FIELDS: { field: NumericField; label: string }[] = [
  { field: "requests_per_day", label: "agents:providerTierDaily" },
  { field: "requests_per_window", label: "agents:providerTierRequestWindow" },
  { field: "request_window_seconds", label: "agents:providerTierRequestWindowSeconds" },
];
type Props = {
  accountId: string;
  tier: BillingTier;
  /** Provider model IDs of the account's free-tier profiles. */
  models: string[];
  state: ProviderAccountLimit["tiers"][BillingTier];
  override: TierLimits;
  onChange: (patch: Partial<TierLimits>) => void;
  disabled: boolean;
};

export function ProviderTierEditor({
  accountId,
  tier,
  models,
  state,
  override,
  onChange,
  disabled,
}: Props) {
  const { t } = useTranslation();
  const fields = tier === "free" ? [...BASE_FIELDS, ...FREE_FIELDS] : BASE_FIELDS;
  const prefix = `account-${accountId}-${tier}`;
  return (
    <fieldset
      className="grid min-w-0 content-start gap-3 rounded-md border p-2"
      data-testid={`account-tier-${accountId}-${tier}`}
      disabled={disabled}
    >
      <legend className="px-1 font-medium">{t(TIER_LABELS[tier])}</legend>
      {fields.map(({ field, label }) => (
        <div key={field} className="grid min-w-0 gap-1">
          <Label htmlFor={`${prefix}-${field}`}>{t(label)}</Label>
          <Input
            id={`${prefix}-${field}`}
            className={settingsControlClassName("w-full")}
            type="number"
            min={0}
            step="any"
            value={override[field] ?? ""}
            placeholder={state.effective[field]?.toString() ?? t("agents:providerTierUnknown")}
            onChange={(event) =>
              onChange({
                [field]: event.target.value === "" ? undefined : Number(event.target.value),
              })
            }
          />
          <span className="break-words text-xs text-muted-foreground">
            {t("agents:providerTierSources", {
              defaults: state.defaults[field] ?? t("agents:providerTierUnknown"),
              learned: state.learned[field] ?? t("agents:providerTierUnknown"),
              override: override[field] ?? t("agents:providerTierInherited"),
            })}
            {state.defaults.provenance?.[field]
              ? ` ${t("agents:providerLimitProvenance", { provenance: t(PROVENANCE_LABELS[state.defaults.provenance[field]]) })}`
              : ""}
          </span>
          {state.effective[field] !== undefined && (
            <span className="text-xs text-muted-foreground">
              {t("agents:providerTierEffective")}: {state.effective[field]}
            </span>
          )}
        </div>
      ))}
      {tier === "free" ? (
        <>
          <div className="grid gap-1">
            <Label htmlFor={`${prefix}-daily-reset`}>{t("agents:providerTierDailyReset")}</Label>
            <Input
              id={`${prefix}-daily-reset`}
              className={settingsControlClassName()}
              value={override.daily_reset ?? ""}
              placeholder={state.effective.daily_reset || t("agents:providerTierDailyResetExample")}
              onChange={(event) => onChange({ daily_reset: event.target.value || undefined })}
            />
            {state.effective.daily_reset && (
              <span className="text-xs text-muted-foreground">
                {t("agents:providerTierEffective")}: {state.effective.daily_reset}
              </span>
            )}
          </div>
          <ProviderTierScopes
            prefix={prefix}
            models={models}
            state={state}
            override={override}
            onChange={onChange}
          />
        </>
      ) : (
        <ProviderTierBudgets
          prefix={prefix}
          tier={tier}
          state={state}
          override={override}
          onChange={onChange}
        />
      )}
      {state.effective.requests_per_model ? (
        <p className="text-xs text-muted-foreground">{t("agents:providerTierPerModel")}</p>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {t("agents:providerTierCurrent", {
          turns: state.usage.active_turns ?? 0,
          minute: state.usage.requests_minute ?? 0,
          day: state.usage.requests_day ?? 0,
        })}
      </p>
    </fieldset>
  );
}
