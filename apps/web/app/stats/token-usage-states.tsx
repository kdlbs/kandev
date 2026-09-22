"use client";

import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import type { TokenUsageResponse } from "@/lib/types/http";

export function TokenUsageLoading() {
  return (
    <div className="space-y-4" data-testid="token-usage-loading" aria-busy="true">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {["one", "two", "three", "four"].map((key) => (
          <Card key={key} className="rounded-sm">
            <CardContent className="space-y-3 p-4">
              <div className="h-3 w-24 animate-pulse rounded bg-muted" />
              <div className="h-7 w-28 animate-pulse rounded bg-muted" />
              <div className="h-3 w-36 animate-pulse rounded bg-muted" />
            </CardContent>
          </Card>
        ))}
      </div>
      <Card className="rounded-sm">
        <CardContent className="p-4">
          <div className="h-40 animate-pulse rounded bg-muted" />
        </CardContent>
      </Card>
    </div>
  );
}

export function TokenUsageError({ message, onRetry }: { message: string; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <Card className="rounded-sm" data-testid="token-usage-error">
      <CardContent className="flex flex-col items-start gap-3 p-6">
        <div>
          <h2 className="text-sm font-medium">{t("stats:failedToLoadTokenUsage")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{message}</p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="min-h-11 cursor-pointer md:min-h-0"
          onClick={onRetry}
        >
          {t("stats:retry")}
        </Button>
      </CardContent>
    </Card>
  );
}

export function TokenUsageEmpty({
  hasHistory,
  onAllTime,
  pluginSettingsHref = "/settings/plugins",
}: {
  hasHistory: boolean;
  onAllTime: () => void;
  pluginSettingsHref?: string;
}) {
  const { t } = useTranslation();
  return (
    <Card
      className="rounded-sm"
      data-testid={hasHistory ? "token-usage-empty-range" : "token-usage-empty-history"}
    >
      <CardContent className="flex flex-col items-start gap-3 p-6">
        <div>
          <h2 className="text-sm font-medium">
            {hasHistory ? t("stats:noTokenUsageInRange") : t("stats:noTokenUsageYet")}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {hasHistory ? t("stats:tryAllTimeRange") : t("stats:tokenUsageSetupHint")}
          </p>
        </div>
        {hasHistory ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="min-h-11 cursor-pointer md:min-h-0"
            onClick={onAllTime}
          >
            {t("stats:rangeAllTime")}
          </Button>
        ) : (
          <Button
            asChild
            variant="outline"
            size="sm"
            className="min-h-11 cursor-pointer md:min-h-0"
          >
            <Link href={pluginSettingsHref}>{t("stats:openPluginSettings")}</Link>
          </Button>
        )}
      </CardContent>
    </Card>
  );
}

export function TokenUsageCoverageNotice({ response }: { response: TokenUsageResponse }) {
  const { t } = useTranslation();
  if (!response.undated_coverage) return null;
  return (
    <div className="rounded-sm border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-800 dark:text-amber-200">
      {t(response.dated_coverage ? "stats:mixedCoverageNotice" : "stats:undatedCoverageNotice")}
    </div>
  );
}
