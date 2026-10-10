import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";
import { formatNumber } from "@/lib/i18n/formats";
import type {
  StorageTemporaryArtifactsSummary,
  StorageTemporaryEntryBreakdown,
  StorageTemporaryEntry,
  StorageTemporaryRootMeasurement,
} from "@/lib/types/system";
import { StorageActionButton } from "./storage-action-button";
import type { Translate } from "./storage-overview-resources";

interface Props {
  roots: StorageTemporaryRootMeasurement[];
  temporaryArtifacts?: StorageTemporaryArtifactsSummary | null;
  onReviewCleanup: () => void;
}

function entryKindLabel(t: Translate, entry: StorageTemporaryEntry): string {
  switch (entry.kind) {
    case "file":
      return t("system:storageTemporaryEntryFile");
    case "directory":
      return t("system:storageTemporaryEntryDirectory");
    case "symlink":
      return t("system:storageTemporaryEntrySymlink");
    case "other":
      return t("system:storageTemporaryEntryOther");
  }
}

function entryOwnershipLabel(t: Translate, ownership: StorageTemporaryEntry["ownership"]): string {
  switch (ownership) {
    case "registered_kandev":
      return t("system:storageTemporaryEntryRegistered");
    case "untracked":
      return t("system:storageTemporaryEntryUntracked");
    case "unknown":
      return t("system:storageTemporaryEntryOwnershipUnknown");
  }
}

function entrySize(t: Translate, sizeBytes: number | undefined): string {
  if (sizeBytes === undefined || !Number.isFinite(sizeBytes) || sizeBytes < 0) {
    return t("system:storageTemporaryEntriesUnavailableValue");
  }
  return t("system:storageTemporaryEntrySize", { size: formatNumber(sizeBytes) });
}

function cleanupStatusText(
  t: Translate,
  summary: StorageTemporaryArtifactsSummary | null | undefined,
): string {
  const staleCount = summary?.stale_count;
  if (
    summary?.available === false ||
    typeof staleCount !== "number" ||
    !Number.isSafeInteger(staleCount) ||
    staleCount < 0
  ) {
    return t("system:storageTemporaryCleanupStatusUnavailable");
  }
  if (staleCount === 0) return t("system:storageTemporaryCleanupNoCandidates");
  return t("system:storageTemporaryCleanupCandidates", {
    count: staleCount,
    size: entrySize(t, summary?.stale_bytes),
  });
}

function TemporaryEntrySummary({
  breakdown,
  t,
}: {
  breakdown: StorageTemporaryEntryBreakdown;
  t: Translate;
}) {
  return (
    <>
      {breakdown.other_observed_count > 0 && (
        <p
          className="break-words text-xs text-muted-foreground"
          data-testid="storage-temporary-other-observed"
        >
          {t("system:storageTemporaryOtherObserved", {
            count: breakdown.other_observed_count,
            size: entrySize(t, breakdown.other_observed_bytes),
          })}
        </p>
      )}
      {breakdown.status === "partial" && (
        <p className="break-words text-xs text-amber-700">
          {t("system:storageTemporaryEntriesPartial")}
        </p>
      )}
    </>
  );
}

function TemporaryEntryList({
  root,
  breakdown,
  t,
}: {
  root: StorageTemporaryRootMeasurement;
  breakdown: StorageTemporaryEntryBreakdown;
  t: Translate;
}) {
  const summary = <TemporaryEntrySummary breakdown={breakdown} t={t} />;
  if (breakdown.entries.length === 0) {
    return (
      <>
        <p className="text-xs text-muted-foreground">{t("system:storageTemporaryEntriesEmpty")}</p>
        {summary}
      </>
    );
  }

  return (
    <>
      <ul className="min-w-0 divide-y rounded-md border">
        {breakdown.entries.map((entry) => (
          <li
            key={`${root.path}/${entry.name}`}
            className="grid min-w-0 gap-1 px-3 py-2 md:grid-cols-[minmax(0,1fr)_8rem_minmax(10rem,0.7fr)] md:items-center md:gap-3"
            data-testid="storage-temporary-entry"
          >
            <span className="min-w-0 break-all text-sm" data-testid="storage-temporary-entry-name">
              {entry.name}
            </span>
            <span
              className="text-xs text-muted-foreground"
              data-testid="storage-temporary-entry-size"
            >
              {entrySize(t, entry.size_bytes)}
            </span>
            <span className="flex min-w-0 flex-wrap items-center gap-1 text-xs text-muted-foreground">
              <span>{entryKindLabel(t, entry)}</span>
              <span data-testid="storage-temporary-entry-ownership">
                {entryOwnershipLabel(t, entry.ownership)}
              </span>
              {entry.completeness === "partial" && (
                <Badge variant="outline" className="text-[10px] font-normal">
                  {t("system:storageTemporaryEntryPartial")}
                </Badge>
              )}
            </span>
          </li>
        ))}
      </ul>
      {summary}
    </>
  );
}

function TemporaryRootEntries({ root }: { root: StorageTemporaryRootMeasurement }) {
  const { t } = useTranslation();
  const breakdown = root.breakdown;
  const unavailable =
    !breakdown || breakdown.status === "unavailable" || breakdown.status === "not_applicable";
  return (
    <section className="min-w-0 space-y-2" data-testid="storage-temporary-root-entries">
      <h4 className="break-all text-sm font-medium">{root.path}</h4>
      {unavailable ? (
        <p className="text-xs text-muted-foreground">
          {t("system:storageTemporaryEntriesUnavailable")}
        </p>
      ) : (
        <TemporaryEntryList root={root} breakdown={breakdown} t={t} />
      )}
    </section>
  );
}

export function StorageTemporaryEntries({ roots, temporaryArtifacts, onReviewCleanup }: Props) {
  const { t } = useTranslation();
  return (
    <div className="mt-3 min-w-0 space-y-4" data-testid="storage-temporary-entries">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">{t("system:storageTemporaryEntriesTitle")}</h3>
        <p className="break-words text-xs text-muted-foreground">
          {t("system:storageTemporaryEntriesDescription")}
        </p>
      </div>
      {roots.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("system:storageTemporaryEntriesUnavailable")}
        </p>
      ) : (
        <div className="space-y-4">
          {roots.map((root) => (
            <TemporaryRootEntries key={`${root.requested_path}:${root.path}`} root={root} />
          ))}
        </div>
      )}
      <div className="space-y-1 text-xs text-muted-foreground">
        <p className="break-words">{t("system:storageTemporaryEntriesSizeNote")}</p>
        <p className="break-words">{t("system:storageTemporaryEntriesScopeNote")}</p>
      </div>
      <p className="break-words text-xs text-muted-foreground">
        {cleanupStatusText(t, temporaryArtifacts)}
      </p>
      <p className="break-words text-xs text-muted-foreground">
        {t("system:storageTemporaryCleanupCapacityNote")}
      </p>
      <StorageActionButton
        variant="outline"
        className="mt-1 h-11 w-full md:h-7 md:w-auto"
        onClick={onReviewCleanup}
        data-testid="storage-temporary-review-cleanup"
        focusId="temporary-review-cleanup"
      >
        {t("system:storageTemporaryReviewCleanup")}
      </StorageActionButton>
    </div>
  );
}
