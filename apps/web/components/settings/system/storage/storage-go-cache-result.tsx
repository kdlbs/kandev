import { useTranslation } from "react-i18next";
import { formatGigabytes } from "./storage-units";

export interface StorageGoCacheResultSummary {
  reclaimedBytes: number;
  bytesAfter: number | null;
  partial: boolean;
  skipped: boolean;
  otherProvidersSkipped: boolean;
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null
    ? (value as Record<string, unknown>)
    : undefined;
}

export function storageGoCacheResultSummary(
  result: Record<string, unknown>,
): StorageGoCacheResultSummary | null {
  const outer = recordValue(result.result) ?? result;
  const provider = recordValue(outer.go_cache);
  const cleanup = recordValue(provider?.result) ?? provider;
  if (!cleanup) return null;
  const reclaimedBytes = cleanup.reclaimed_bytes;
  if (
    typeof reclaimedBytes !== "number" ||
    !Number.isFinite(reclaimedBytes) ||
    reclaimedBytes < 0
  ) {
    return null;
  }
  const rawBytesAfter = cleanup.bytes_after;
  const bytesAfter =
    typeof rawBytesAfter === "number" && Number.isFinite(rawBytesAfter) && rawBytesAfter >= 0
      ? rawBytesAfter
      : null;
  const skippedProviders = recordValue(outer.skipped_providers);
  return {
    reclaimedBytes,
    bytesAfter,
    partial:
      cleanup.partial === true || (Array.isArray(cleanup.errors) && cleanup.errors.length > 0),
    skipped: cleanup.skipped === true,
    otherProvidersSkipped: Boolean(skippedProviders && Object.keys(skippedProviders).length > 0),
  };
}

export function StorageGoCacheResult({
  result,
  busyPolicyEnabled,
  testId = "storage-go-cache-result",
}: {
  result: Record<string, unknown>;
  busyPolicyEnabled: boolean;
  testId?: string;
}) {
  const { t } = useTranslation();
  const cleanup = storageGoCacheResultSummary(result);
  if (!cleanup) return null;
  return (
    <div className="min-w-0 space-y-1 text-sm" data-testid={testId}>
      {cleanup.skipped ? (
        <p>{t("system:storageGoCacheSkippedResult")}</p>
      ) : (
        <p>
          {t("system:storageGoCacheRemovedResult", {
            size: formatGigabytes(cleanup.reclaimedBytes),
          })}
        </p>
      )}
      <p className="text-muted-foreground">
        {cleanup.bytesAfter === null
          ? t("system:storageGoCacheRemainingUnknown")
          : t("system:storageGoCacheRemainingResult", {
              size: formatGigabytes(cleanup.bytesAfter),
            })}
      </p>
      {busyPolicyEnabled && <p>{t("system:storageGoCacheBusyPolicySnapshot")}</p>}
      {cleanup.partial && (
        <p className="text-amber-600">{t("system:storageGoCachePartialResult")}</p>
      )}
      {cleanup.otherProvidersSkipped && (
        <p className="text-amber-600">{t("system:storageOtherCleanupSkippedBusy")}</p>
      )}
    </div>
  );
}
