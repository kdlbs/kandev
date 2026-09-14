"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { formatTimeDistance, useDateLocale } from "@/lib/i18n/date-locale";
import type { GitHubPRDiscoveryHealth } from "@/lib/types/github";
import { useTranslation } from "react-i18next";

const CATEGORY_LABEL_KEYS: Record<NonNullable<GitHubPRDiscoveryHealth["category"]>, string> = {
  invalid_query: "github:prDiscoveryCategoryInvalidQuery",
  rate_limited: "github:prDiscoveryCategoryRateLimited",
  unavailable: "github:prDiscoveryCategoryUnavailable",
};

function isDegraded(health?: GitHubPRDiscoveryHealth): health is GitHubPRDiscoveryHealth {
  return Boolean(health && health.state === "degraded" && health.failed_target_count > 0);
}

function relativeTime(
  value: string | undefined,
  locale: ReturnType<typeof useDateLocale>,
  fallback: string,
) {
  return formatTimeDistance(value, locale) || fallback;
}

export function GitHubPRDiscoveryHealthWarning({ health }: { health?: GitHubPRDiscoveryHealth }) {
  const { t } = useTranslation();
  const locale = useDateLocale();
  if (!isDegraded(health)) return null;
  const category = health.category
    ? t(CATEGORY_LABEL_KEYS[health.category])
    : t("github:prDiscoveryCategoryUnavailable");
  const unknownTime = t("github:prDiscoveryUnknownTime");
  return (
    <Alert variant="destructive" className="min-w-0" data-testid="github-pr-discovery-health">
      <IconAlertTriangle className="h-4 w-4 shrink-0" />
      <AlertDescription className="min-w-0 text-xs">
        <div className="space-y-1 break-words">
          <p className="font-medium">{t("github:prDiscoveryFailed")}</p>
          <p>{t("github:prDiscoveryCategory", { category })}</p>
          <p>
            {t("github:prDiscoveryLastFailure", {
              time: relativeTime(health.last_failure_at, locale, unknownTime),
            })}
          </p>
          <p>{t("github:prDiscoveryAffectedTargets", { count: health.failed_target_count })}</p>
          {health.retry_at && (
            <p>
              {t("github:prDiscoveryRetryAt", {
                time: relativeTime(health.retry_at, locale, unknownTime),
              })}
            </p>
          )}
        </div>
      </AlertDescription>
    </Alert>
  );
}

export function GitHubPRDiscoveryHealthDetails({ health }: { health?: GitHubPRDiscoveryHealth }) {
  return <GitHubPRDiscoveryHealthWarning health={health} />;
}
