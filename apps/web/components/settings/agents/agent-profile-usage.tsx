"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useAgentProfileUsage } from "@/hooks/domains/settings/use-agent-profile-usage";
import { formatDateTime, formatNumber } from "@/lib/i18n/formats";
import type { AgentProfileUsage } from "@/lib/api/domains/agent-profile-usage-api";

export function AgentProfileUsageLine({
  profileId,
  dynamic,
}: {
  profileId: string;
  dynamic: boolean;
}) {
  const enabled = useFeature("providerUsageLimits");
  if (!enabled || dynamic) return null;
  return <EnabledUsageLine profileId={profileId} />;
}

function EnabledUsageLine({ profileId }: { profileId: string }) {
  const { t } = useTranslation();
  const { usage, loading, loaded, failed, refresh } = useAgentProfileUsage(profileId);
  let content = <UsageReading usage={usage} />;
  if (!loaded || loading) content = <div>{t("common:loading")}</div>;
  else if (failed) content = <div>{t("agents:providerUsageUnavailable")}</div>;
  return (
    <div
      className="relative z-10 mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 pl-3.5 text-xs text-muted-foreground"
      data-testid="agent-profile-usage"
    >
      <div className="min-w-0 flex-1 break-words" aria-live="polite">
        <span className="font-medium">{t("agents:providerUsageTitle")}</span>
        {content}
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="min-h-11 shrink-0"
        disabled={loading}
        aria-label={t("common:refreshAriaLabel", { label: t("agents:providerUsageTitle") })}
        onClick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          void refresh();
        }}
      >
        {t("agents:refresh")}
      </Button>
    </div>
  );
}

function UsageReading({ usage }: { usage?: AgentProfileUsage }) {
  const { t } = useTranslation();
  if (!usage) return <div>{t("agents:providerUsageUnsupported")}</div>;
  const classKeys = {
    free: "agents:providerUsageFree",
    paid: "agents:providerUsagePaid",
    subscription: "agents:providerUsageSubscription",
  } as const;
  let state = t("agents:providerUsageUnsupported");
  if (usage.state === "no_usage_api") state = t("agents:providerUsageNoApi");
  if (usage.state === "unavailable") {
    state =
      usage.reason === "credential_missing"
        ? t("agents:capabilityAuthRequired")
        : t("agents:providerUsageUnavailable");
  }
  return (
    <>
      <div className="break-all">
        {usage.provider}
        {usage.model_class && <> · {t(classKeys[usage.model_class])}</>}
        {usage.account_id && (
          <> · {t("agents:providerUsageAccount", { account: usage.account_id })}</>
        )}
      </div>
      {usage.binding_scope === "host_default" && <div>{t("agents:providerUsageHostDefault")}</div>}
      {usage.state !== "ok" ? (
        <div>{state}</div>
      ) : (
        <>
          {usage.stale && <div role="status">{t("agents:providerUsageStale")}</div>}
          <div>
            {usage.source === "proxy"
              ? t("agents:providerUsageProxy")
              : t("agents:providerUsageProvider")}
          </div>
          {usage.windows.map((window, index) => (
            <div key={`${window.label}-${index}`}>
              {t("agents:providerUsageWindow", {
                window: window.label,
                percent: formatNumber(window.utilization_pct, { maximumFractionDigits: 1 }),
                reset: window.reset_at ? formatDateTime(window.reset_at) : t("common:unknown"),
              })}
              {window.limit_reached && <> · {t("agents:providerUsageLimitReached")}</>}
            </div>
          ))}
          {usage.fetched_at && (
            <div>{t("agents:providerUsageReadAt", { time: formatDateTime(usage.fetched_at) })}</div>
          )}
        </>
      )}
    </>
  );
}
