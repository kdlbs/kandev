import { useTranslation } from "react-i18next";
import {
  storageGoCacheResultSummary,
  type StorageHistoryIssueCategory,
} from "./storage-run-summary";
import { formatGigabytes } from "./storage-units";

const ISSUE_EXPLANATION_KEYS: Record<StorageHistoryIssueCategory, string> = {
  entries_changed: "system:storageHistoryEntriesChanged",
  entries_preserved: "system:storageHistoryEntriesPreserved",
  unreadable: "system:storageHistoryUnreadable",
  delete_failed: "system:storageHistoryDeleteFailed",
  boundary_preserved: "system:storageHistoryBoundaryPreserved",
  entry_limit: "system:storageHistoryEntryLimit",
  deadline: "system:storageHistoryDeadline",
  cancelled: "system:storageHistoryCancelled",
  other: "system:storageHistoryOtherError",
  unknown: "system:storageHistoryUnknownError",
};

export function StorageGoCacheResult({
  result,
  message,
  busyPolicyEnabled,
  testId = "storage-go-cache-result",
}: {
  result: Record<string, unknown>;
  message?: string;
  busyPolicyEnabled: boolean;
  testId?: string;
}) {
  const { t } = useTranslation();
  const cleanup = storageGoCacheResultSummary(result, message);
  if (!cleanup) return null;
  return (
    <div className="min-w-0 space-y-2 text-sm" data-testid={testId}>
      {cleanup.skipped ? (
        <div>{t("system:storageGoCacheSkippedResult")}</div>
      ) : (
        <div className="flex flex-col gap-1 sm:flex-row sm:flex-wrap sm:gap-x-6">
          <div data-testid="storage-go-cache-removed">
            {t("system:storageGoCacheRemovedFact", {
              size: formatGigabytes(cleanup.reclaimedBytes),
            })}
          </div>
          <div className="text-muted-foreground" data-testid="storage-go-cache-remaining">
            {cleanup.bytesAfter === null
              ? t("system:storageGoCacheRemainingUnknownFact")
              : t("system:storageGoCacheRemainingFact", {
                  size: formatGigabytes(cleanup.bytesAfter),
                })}
          </div>
        </div>
      )}
      {busyPolicyEnabled && <div>{t("system:storageGoCacheBusyPolicySnapshot")}</div>}
      {cleanup.issueCategories.map((category) => (
        <div className="text-amber-600" key={category} data-testid="storage-go-cache-explanation">
          {t(ISSUE_EXPLANATION_KEYS[category])}
        </div>
      ))}
      {cleanup.partial && cleanup.issueCategories.length === 0 && (
        <div className="text-amber-600" data-testid="storage-go-cache-partial-explanation">
          {t("system:storageGoCachePartialResult")}
        </div>
      )}
      {cleanup.otherProvidersSkipped && (
        <div className="text-amber-600">{t("system:storageOtherCleanupSkippedBusy")}</div>
      )}
    </div>
  );
}
