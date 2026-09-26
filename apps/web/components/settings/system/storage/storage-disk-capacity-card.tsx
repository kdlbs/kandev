import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Progress } from "@kandev/ui/progress";
import { Spinner } from "@kandev/ui/spinner";
import { IconDatabase } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { formatNumber } from "@/lib/i18n/formats";
import type { StorageDiskCapacityResponse } from "@/lib/types/system";
import { formatGigabytes } from "./storage-units";

type Props = {
  disk: StorageDiskCapacityResponse | null;
  loading?: boolean;
  error?: string | null;
};

type Severity = "normal" | "warning" | "critical";

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(100, Math.max(0, value));
}

function severityForPercent(percent: number): Severity {
  if (percent >= 90) return "critical";
  if (percent >= 80) return "warning";
  return "normal";
}

function severityTextClassName(severity: Severity): string {
  switch (severity) {
    case "critical":
      return "text-lg font-semibold text-red-600 dark:text-red-400";
    case "warning":
      return "text-lg font-semibold text-amber-600 dark:text-amber-400";
    default:
      return "text-lg font-semibold";
  }
}

function severityProgressClassName(severity: Severity): string {
  switch (severity) {
    case "critical":
      return "h-3 [&_[data-slot=progress-indicator]]:bg-red-500";
    case "warning":
      return "h-3 [&_[data-slot=progress-indicator]]:bg-amber-500";
    default:
      return "h-3";
  }
}

function LoadingDiskCard({ loading, error }: Pick<Props, "loading" | "error">) {
  const { t } = useTranslation();
  const isLoading = loading ?? true;
  return (
    <Card data-testid="storage-disk-capacity-card">
      <CardContent className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
        {isLoading && <Spinner className="size-4" data-testid="storage-disk-spinner" />}
        <span>
          {isLoading ? t("system:storageDiskLoading") : t("system:storageSectionUnavailable")}
        </span>
        {error && <span className="break-words text-destructive">{error}</span>}
      </CardContent>
    </Card>
  );
}

function UnavailableDiskCard({ disk }: { disk: StorageDiskCapacityResponse }) {
  const { t } = useTranslation();
  return (
    <Card data-testid="storage-disk-capacity-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <IconDatabase className="size-4" /> {t("system:storageDiskTitle")}
        </CardTitle>
        <CardDescription>{t("system:storageDiskDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground" data-testid="storage-disk-unavailable">
          {t("system:storageDiskUnavailable")}
        </p>
        {disk.path && (
          <code className="mt-2 block break-all text-xs text-muted-foreground">{disk.path}</code>
        )}
      </CardContent>
    </Card>
  );
}

function AvailableDiskCard({
  disk,
  loading,
  error,
}: {
  disk: StorageDiskCapacityResponse;
  loading: boolean;
  error?: string | null;
}) {
  const { t } = useTranslation();
  const percent = clampPercent(disk.used_percent);
  const severity = severityForPercent(percent);
  const percentLabel = formatNumber(percent, { maximumFractionDigits: 1 });
  const progressLabel = t("system:storageDiskProgressAria", { percent: percentLabel });
  return (
    <Card data-testid="storage-disk-capacity-card" data-severity={severity}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <IconDatabase className="size-4" /> {t("system:storageDiskTitle")}
          {loading && <Spinner className="size-4" data-testid="storage-disk-spinner" />}
        </CardTitle>
        <CardDescription>{t("system:storageDiskDescription")}</CardDescription>
        {error && (
          <p className="break-words text-xs text-destructive" data-testid="storage-disk-error">
            {t("system:storageSectionUnavailable")}: {error}
          </p>
        )}
      </CardHeader>
      <CardContent className="min-w-0 space-y-3">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <span className={severityTextClassName(severity)} data-testid="storage-disk-used-percent">
            {t("system:storageDiskUsed", { percent: percentLabel })}
          </span>
          <span className="text-xs text-muted-foreground">
            {t("system:storageDiskUsedSize", { size: formatGigabytes(disk.used_bytes) })}
          </span>
        </div>
        <Progress
          value={percent}
          aria-label={progressLabel}
          aria-valuemin={0}
          aria-valuemax={100}
          className={severityProgressClassName(severity)}
        />
        <div className="flex min-w-0 flex-wrap justify-between gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span>
            {t("system:storageDiskCapacitySummary", {
              available: formatGigabytes(disk.available_bytes),
              total: formatGigabytes(disk.total_bytes),
            })}
          </span>
        </div>
        <div className="min-w-0 text-xs text-muted-foreground">
          <span>{t("system:storageDiskPath")}: </span>
          <code className="break-all" data-testid="storage-disk-capacity-path">
            {disk.path}
          </code>
        </div>
        {severity !== "normal" && (
          <p
            className={
              severity === "critical"
                ? "text-xs text-red-600 dark:text-red-400"
                : "text-xs text-amber-600 dark:text-amber-400"
            }
            data-testid="storage-disk-threshold-warning"
          >
            {severity === "critical"
              ? t("system:storageDiskCriticalWarning")
              : t("system:storageDiskHighWarning")}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

export function StorageDiskCapacityCard({ disk, loading, error }: Props) {
  if (!disk) return <LoadingDiskCard loading={loading} error={error} />;
  if (!disk.available) return <UnavailableDiskCard disk={disk} />;
  return <AvailableDiskCard disk={disk} loading={loading ?? false} error={error} />;
}
