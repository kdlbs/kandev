"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconGauge } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useProviderLimits } from "@/hooks/domains/settings/use-provider-limits";
import type { ProviderLimit } from "@/lib/api/domains/provider-limits-api";
import { formatDateTime } from "@/lib/i18n/formats";
import {
  browserTimeZone,
  fromLocalInputValue,
  toLocalInputValue,
} from "@/lib/settings/provider-limits";
import { settingsActionClassName } from "./settings-control";
import { ProviderAccountLimits } from "./provider-account-limits";

type ProviderLimitsCardProps = {
  /** Show only this provider, for a profile whose model belongs to it. */
  provider?: string;
};

/**
 * Operator-entered provider limits for dynamic routing: the monthly reset used
 * when the provider does not report one, and a manual block of every model of
 * the provider until a time seen on its console.
 */
export function ProviderLimitsCard({ provider }: ProviderLimitsCardProps) {
  const limitsEnabled = useFeature("providerUsageLimits");
  if (!limitsEnabled) return null;
  return <ProviderLimitsCardBody provider={provider} />;
}

function ProviderLimitsCardBody({ provider }: ProviderLimitsCardProps) {
  const { t } = useTranslation();
  const { state, save } = useProviderLimits();
  const providers =
    state.status === "ready"
      ? state.providers.filter((entry) => !provider || entry.provider === provider)
      : [];
  if (provider && state.status === "ready" && providers.length === 0) return null;

  return (
    <Card className="min-w-0 gap-0 py-0" data-testid="provider-limits-card">
      <CardHeader className="px-3 py-3">
        <CardTitle className="flex items-center gap-2 text-lg">
          <IconGauge className="h-5 w-5 shrink-0" aria-hidden />
          {t("agents:providerLimitsTitle")}
        </CardTitle>
        <p className="text-sm text-muted-foreground">{t("agents:providerLimitsDescription")}</p>
      </CardHeader>
      <CardContent className="grid gap-3 px-3 pb-3">
        {state.status === "loading" ? (
          <p className="text-xs text-muted-foreground" role="status">
            {t("agents:providerLimitsLoading")}
          </p>
        ) : null}
        {state.status === "failed" ? (
          <p className="text-xs text-destructive">{t("agents:providerLimitsLoadFailed")}</p>
        ) : null}
        {state.status === "ready" && providers.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("agents:providerLimitsEmpty")}</p>
        ) : null}
        {providers.map((entry) => (
          <ProviderLimitRow key={entry.provider} limit={entry} onSave={save} />
        ))}
      </CardContent>
    </Card>
  );
}

type SaveProviderLimit = ReturnType<typeof useProviderLimits>["save"];

function ProviderLimitRow({ limit, onSave }: { limit: ProviderLimit; onSave: SaveProviderLimit }) {
  const { t } = useTranslation();
  const [monthly, setMonthly] = useState(toLocalInputValue(limit.monthly_reset_at));
  const [blockUntil, setBlockUntil] = useState(toLocalInputValue(limit.block_until));
  const [status, setStatus] = useState<"idle" | "saving" | "saved" | "failed">("idle");

  const submit = async (nextMonthly: string, nextBlock: string) => {
    setStatus("saving");
    try {
      await onSave(limit.provider, {
        monthly_reset_at: fromLocalInputValue(nextMonthly),
        monthly_reset_timezone: nextMonthly ? browserTimeZone() : "",
        block_until: fromLocalInputValue(nextBlock),
      });
      setStatus("saved");
    } catch {
      setStatus("failed");
    }
  };

  const monthlyId = `provider-limit-monthly-${limit.provider}`;
  const blockId = `provider-limit-block-${limit.provider}`;
  return (
    <div
      className="grid gap-2 rounded-md border p-3"
      data-testid={`provider-limit-${limit.provider}`}
    >
      <ProviderLimitSummary limit={limit} />
      {limit.accounts?.map((account) => (
        <ProviderAccountLimits
          key={account.account_id}
          account={account}
          provider={limit.provider}
          save={onSave}
        />
      ))}
      <div className="grid gap-2 sm:grid-cols-2">
        <div className="grid gap-1">
          <Label htmlFor={monthlyId}>{t("agents:providerLimitsMonthlyReset")}</Label>
          <Input
            id={monthlyId}
            type="datetime-local"
            value={monthly}
            onChange={(event) => setMonthly(event.target.value)}
          />
          <span className="text-xs text-muted-foreground">
            {t("agents:providerLimitsMonthlyResetHint")}
          </span>
        </div>
        <div className="grid gap-1">
          <Label htmlFor={blockId}>{t("agents:providerLimitsBlockUntil")}</Label>
          <Input
            id={blockId}
            type="datetime-local"
            value={blockUntil}
            onChange={(event) => setBlockUntil(event.target.value)}
          />
          <span className="text-xs text-muted-foreground">
            {t("agents:providerLimitsBlockHint")}
          </span>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          className={settingsActionClassName("cursor-pointer")}
          disabled={status === "saving"}
          onClick={() => void submit(monthly, blockUntil)}
        >
          {t("agents:providerLimitsSave")}
        </Button>
        <Button
          variant="ghost"
          className={settingsActionClassName("cursor-pointer")}
          disabled={status === "saving" || (!monthly && !blockUntil)}
          onClick={() => {
            setMonthly("");
            setBlockUntil("");
            void submit("", "");
          }}
        >
          {t("agents:providerLimitsClear")}
        </Button>
        {status === "saved" ? (
          <span className="text-xs text-muted-foreground" role="status">
            {t("agents:providerLimitsSaved")}
          </span>
        ) : null}
        {status === "failed" ? (
          <span className="text-xs text-destructive" role="alert">
            {t("agents:providerLimitsSaveFailed")}
          </span>
        ) : null}
      </div>
    </div>
  );
}

function ProviderLimitSummary({ limit }: { limit: ProviderLimit }) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{limit.provider}</span>
        <Badge variant="outline">
          {t("agents:providerLimitsProfileCount", { count: limit.profile_count })}
        </Badge>
        {limit.model_scoped ? (
          <Badge variant="secondary">{t("agents:providerLimitsModelScoped")}</Badge>
        ) : null}
      </div>
      {limit.next_monthly_reset ? (
        <span className="text-xs text-muted-foreground">
          {t(
            limit.next_monthly_reset_source === "usage"
              ? "agents:providerLimitsNextResetUsage"
              : "agents:providerLimitsNextResetManual",
            { time: formatDateTime(limit.next_monthly_reset) },
          )}
        </span>
      ) : null}
      {limit.observed_exhausted_until ? (
        <span className="text-xs text-muted-foreground">
          {t("agents:providerLimitsExhaustedUntil", {
            time: formatDateTime(limit.observed_exhausted_until),
          })}
        </span>
      ) : null}
    </div>
  );
}
