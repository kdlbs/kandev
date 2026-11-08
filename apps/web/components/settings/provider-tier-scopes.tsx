"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type {
  ProviderAccountLimit,
  RequestScope,
  TierLimits,
} from "@/lib/api/domains/provider-limits-api";
import {
  PROVENANCE_LABELS,
  scopeRows,
  updateScopeOverride,
} from "@/lib/settings/provider-tier-limits";
import { settingsActionClassName, settingsControlClassName } from "./settings-control";
import { ProviderTierScopeAdder } from "./provider-tier-scope-adder";

const UNKNOWN = "agents:providerTierUnknown";

type TierState = ProviderAccountLimit["tiers"]["free"];
type Props = {
  prefix: string;
  models: string[];
  state: TierState;
  override: TierLimits;
  onChange: (patch: Partial<TierLimits>) => void;
};

/** Model and shared-group daily scopes of a free tier, under its account-wide value. */
export function ProviderTierScopes({ prefix, models, state, override, onChange }: Props) {
  const { t } = useTranslation();
  const overrides = override.scopes ?? [];
  const rows = scopeRows(state.effective, override);
  return (
    <div className="grid min-w-0 gap-2 border-t pt-2" data-testid={`${prefix}-scopes`}>
      <p className="font-medium text-sm">{t("agents:providerScopesTitle")}</p>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("agents:providerScopesEmpty")}</p>
      ) : null}
      {rows.map((scope) => (
        <ProviderScopeRow
          key={scope.id}
          id={`${prefix}-scope-${scope.id}`}
          scope={scope}
          state={state}
          override={overrides.find((entry) => entry.id === scope.id)}
          onValue={(value) => onChange({ scopes: updateScopeOverride(overrides, scope, value) })}
          onRemove={() => onChange({ scopes: overrides.filter((entry) => entry.id !== scope.id) })}
        />
      ))}
      <ProviderTierScopeAdder
        prefix={prefix}
        models={models}
        existing={rows}
        onAdd={(scope) => onChange({ scopes: [...overrides, scope] })}
      />
    </div>
  );
}

type RowProps = {
  id: string;
  scope: RequestScope;
  state: TierState;
  override?: RequestScope;
  onValue: (value: number | undefined) => void;
  onRemove: () => void;
};

function ProviderScopeRow({ id, scope, state, override, onValue, onRemove }: RowProps) {
  const { t } = useTranslation();
  const defaults = state.defaults.scopes?.find((entry) => entry.id === scope.id);
  const learned = state.learned.scopes?.find((entry) => entry.id === scope.id);
  const kindLabel =
    scope.kind === "group" ? "agents:providerScopeGroup" : "agents:providerScopeModel";
  const ownerDefined = Boolean(override?.kind) && !defaults && !learned;
  return (
    <div className="grid min-w-0 gap-1" data-testid={id}>
      <Label htmlFor={`${id}-value`} className="break-all">
        {t(kindLabel, { models: (scope.models ?? []).join(", ") })}
      </Label>
      <ScopeValueInput id={`${id}-value`} scope={scope} override={override} onValue={onValue} />
      <ScopeSources defaults={defaults} learned={learned} override={override} />
      <ScopeStatus
        scope={scope}
        used={state.usage.scopes?.[scope.id] ?? 0}
        provenance={defaults?.provenance}
        provisional={learned?.provisional}
      />
      {ownerDefined ? (
        <Button
          type="button"
          variant="ghost"
          className={settingsActionClassName("justify-self-start")}
          onClick={onRemove}
        >
          {t("agents:providerScopeRemove")}
        </Button>
      ) : null}
    </div>
  );
}

function ScopeValueInput({
  id,
  scope,
  override,
  onValue,
}: {
  id: string;
  scope: RequestScope;
  override?: RequestScope;
  onValue: (value: number | undefined) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Input
        id={id}
        className={settingsControlClassName("w-full")}
        type="number"
        min={0}
        step="any"
        value={override?.requests_per_day ?? ""}
        placeholder={scope.requests_per_day?.toString() ?? t(UNKNOWN)}
        onChange={(event) =>
          onValue(event.target.value === "" ? undefined : Number(event.target.value))
        }
      />
      {scope.requests_per_day !== undefined && (
        <span className="text-xs text-muted-foreground">
          {t("agents:providerTierEffective")}: {scope.requests_per_day}
        </span>
      )}
    </>
  );
}

function ScopeSources({
  defaults,
  learned,
  override,
}: {
  defaults?: RequestScope;
  learned?: RequestScope;
  override?: RequestScope;
}) {
  const { t } = useTranslation();
  return (
    <span className="break-words text-xs text-muted-foreground">
      {t("agents:providerTierSources", {
        defaults: defaults?.requests_per_day ?? t(UNKNOWN),
        learned: learned?.requests_per_day ?? t(UNKNOWN),
        override: override?.requests_per_day ?? t("agents:providerTierInherited"),
      })}
    </span>
  );
}

function ScopeStatus({
  scope,
  used,
  provenance,
  provisional,
}: {
  scope: RequestScope;
  used: number;
  provenance?: RequestScope["provenance"];
  provisional?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <span className="text-xs text-muted-foreground">
        {t("agents:providerScopeToday", { used, reset: scope.daily_reset || t(UNKNOWN) })}
        {provenance
          ? ` ${t("agents:providerLimitProvenance", { provenance: t(PROVENANCE_LABELS[provenance]) })}`
          : ""}
      </span>
      {provisional ? (
        <p className="text-xs text-muted-foreground">{t("agents:providerScopeProvisional")}</p>
      ) : null}
    </>
  );
}
