"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type {
  BillingTier,
  LimitBudget,
  ProviderAccountLimit,
  TierLimits,
} from "@/lib/api/domains/provider-limits-api";
import { settingsControlClassName } from "./settings-control";
import { budgetKey, updateBudgetOverride } from "@/lib/settings/provider-tier-limits";

const WINDOW_LABELS = {
  "5h": "agents:providerTierFiveHours",
  day: "agents:providerTierDay",
  week: "agents:providerTierWeek",
  month: "agents:providerTierMonth",
};
const UNIT_LABELS = {
  usd: "agents:providerTierUSD",
  tokens: "agents:providerTierTokens",
  percent: "agents:providerTierPercent",
  requests: "agents:providerTierRequests",
  turns: "agents:providerTierTurns",
};
const RESET_LABELS = {
  rolling: "agents:providerTierRolling",
  utc: "agents:providerTierUTC",
  provider: "agents:providerTierProviderReset",
  anchor: "agents:providerTierAnchor",
};
type Props = {
  prefix: string;
  tier: BillingTier;
  state: ProviderAccountLimit["tiers"][BillingTier];
  override: TierLimits;
  onChange: (patch: Partial<TierLimits>) => void;
};

export function ProviderTierBudgets({ prefix, tier, state, override, onChange }: Props) {
  const periods: LimitBudget["window"][] =
    tier === "paid" ? ["day", "month"] : ["5h", "week", "month"];
  const budgets = [...(state.effective.windows ?? [])];
  for (const window of periods)
    if (!budgets.some((budget) => budget.window === window))
      budgets.push({
        window,
        unit: tier === "paid" ? "usd" : "percent",
        reset: tier === "paid" ? "utc" : "provider",
      });
  const update = (budget: LimitBudget, patch: Partial<LimitBudget>, clear = false) => {
    onChange({ windows: updateBudgetOverride(override.windows ?? [], budget, patch, clear) });
  };
  return (
    <>
      {budgets.map((budget) => (
        <ProviderBudgetEditor
          key={budgetKey(budget)}
          prefix={prefix}
          budget={budget}
          state={state}
          override={override.windows?.find((entry) => budgetKey(entry) === budgetKey(budget))}
          onChange={(patch, clear) => update(budget, patch, clear)}
        />
      ))}
    </>
  );
}

function ProviderBudgetEditor({
  prefix,
  budget,
  state,
  override,
  onChange,
}: {
  prefix: string;
  budget: LimitBudget;
  state: Props["state"];
  override?: LimitBudget;
  onChange: (patch: Partial<LimitBudget>, clear?: boolean) => void;
}) {
  const { t } = useTranslation();
  const id = `${prefix}-${budgetKey(budget)}`;
  const effective = { ...budget, ...override };
  const defaults = state.defaults.windows?.find((entry) => budgetKey(entry) === budgetKey(budget));
  const learned = state.learned.windows?.find((entry) => budgetKey(entry) === budgetKey(budget));
  return (
    <div className="grid min-w-0 gap-1 border-t pt-2">
      <Label htmlFor={`${id}-value`}>
        {t(WINDOW_LABELS[budget.window])}
        {budget.scope ? ` (${budget.scope})` : ""}
      </Label>
      <Input
        id={`${id}-value`}
        type="number"
        min={0}
        step="any"
        className={settingsControlClassName()}
        value={override?.value ?? ""}
        placeholder={budget.value?.toString() ?? t("agents:providerTierUnknown")}
        onChange={(event) =>
          onChange(
            { value: event.target.value === "" ? undefined : Number(event.target.value) },
            event.target.value === "",
          )
        }
      />
      <span className="break-words text-xs text-muted-foreground">
        {t("agents:providerTierSources", {
          defaults: defaults?.value ?? t("agents:providerTierUnknown"),
          learned: learned?.value ?? t("agents:providerTierUnknown"),
          override: override?.value ?? t("agents:providerTierInherited"),
        })}
      </span>
      {budget.window_estimated ? (
        <p className="text-xs text-muted-foreground">
          {t("agents:providerTierUnknownWindowLearned")}
        </p>
      ) : null}
      <BudgetRules id={id} effective={effective} onChange={onChange} />
    </div>
  );
}

function BudgetRules({
  id,
  effective,
  onChange,
}: {
  id: string;
  effective: LimitBudget;
  onChange: (patch: Partial<LimitBudget>) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Label htmlFor={`${id}-unit`}>{t("agents:providerTierBudgetUnit")}</Label>
      <Select
        value={effective.unit}
        onValueChange={(unit) => onChange({ unit: unit as LimitBudget["unit"] })}
      >
        <SelectTrigger id={`${id}-unit`} className={settingsControlClassName()}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {Object.entries(UNIT_LABELS).map(([unit, label]) => (
            <SelectItem key={unit} value={unit}>
              {t(label)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Label htmlFor={`${id}-reset`}>{t("agents:providerTierBudgetReset")}</Label>
      <Select
        value={effective.reset}
        onValueChange={(reset) => onChange({ reset: reset as LimitBudget["reset"] })}
      >
        <SelectTrigger id={`${id}-reset`} className={settingsControlClassName()}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {Object.entries(RESET_LABELS).map(([reset, label]) => (
            <SelectItem key={reset} value={reset}>
              {t(label)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {effective.reset === "anchor" || effective.reset === "provider" ? (
        <div className="grid gap-1">
          <Label htmlFor={`${id}-anchor`}>{t("agents:providerTierResetInstant")}</Label>
          <Input
            id={`${id}-anchor`}
            className={settingsControlClassName()}
            value={effective.reset_at ?? ""}
            placeholder={t("agents:providerTierResetInstantHint")}
            onChange={(event) => onChange({ reset_at: event.target.value || undefined })}
          />
        </div>
      ) : null}
    </>
  );
}
