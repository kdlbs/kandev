import type {
  StorageSourceProgress,
  StorageSummaryPartial,
  StorageSystemTemporarySummary,
  StorageTemporaryArtifactsSummary,
} from "@/lib/types/system";
import type { StorageResource, Translate } from "./storage-overview-resources";
import { formatGigabytes } from "./storage-units";

const STORAGE_UNAVAILABLE_VALUE_KEY = "system:storageUnavailableValue";
const SYSTEM_TEMPORARY_RESOURCE_ID = "system-temporary";
// i18n-exempt: stable backend error marker from older storage snapshots.
const LEGACY_DEADLINE_WARNING = "context deadline exceeded";
const TEMPORARY_WARNING_LIMIT = 10;

function measuredBytes(value: number | undefined): number | undefined {
  if (value === undefined || !Number.isFinite(value) || value < 0) return undefined;
  return value;
}

function pendingStorageResource(
  t: Translate,
  id: string,
  label: string,
  progress?: StorageSourceProgress,
  source = id,
): StorageResource {
  let status = t("system:storageAnalysisSourceComplete");
  if (!progress || progress.state === "pending") {
    status = t("system:storageAnalysisSourcePending");
  } else if (progress.state === "scanning") {
    status =
      progress.total_items === undefined
        ? t("system:storageAnalysisSourceScanningUnknown")
        : t("system:storageAnalysisSourceScanning", {
            completed: progress.completed_items,
            total: progress.total_items,
          });
  } else if (progress.state === "failed") {
    status = progress.error
      ? t("system:storageAnalysisSourceFailedWithError", { error: progress.error })
      : t("system:storageAnalysisSourceFailed");
  }
  return { id, label, value: status, detail: status, source };
}

function temporaryRootStatus(
  t: Translate,
  status: StorageSystemTemporarySummary["status"],
): string {
  switch (status) {
    case "measured":
      return t("system:storageSystemTemporaryComplete");
    case "partial":
      return t("system:storageSystemTemporaryPartial");
    case "unavailable":
      return t("system:storageSystemTemporaryUnavailable");
    case "not_applicable":
      return t("system:storageSystemTemporaryNotApplicable");
  }
}

function temporaryRootLine(
  t: Translate,
  root: NonNullable<StorageSystemTemporarySummary["roots"]>[number],
): string {
  const path =
    root.requested_path === root.path
      ? root.path
      : t("system:storageSystemTemporaryResolvedRoot", {
          requested: root.requested_path,
          resolved: root.path,
        });
  let value: string;
  if (root.status === "unavailable" || root.status === "not_applicable") {
    value = temporaryRootStatus(t, root.status);
  } else if (root.size_bytes === undefined) {
    value = t(STORAGE_UNAVAILABLE_VALUE_KEY);
  } else {
    value = formatGigabytes(root.size_bytes);
  }
  const line = t("system:storageSystemTemporaryRoot", {
    path,
    size: value,
    status: temporaryRootStatus(t, root.status),
  });
  if (!root.skipped_count) return line;
  return [line, t("system:storageSystemTemporarySkippedCount", { count: root.skipped_count })].join(
    " · ",
  );
}

function temporarySummaryBytes(summary: StorageSystemTemporarySummary): number | undefined {
  if (summary.status === "unavailable" || summary.status === "not_applicable") {
    return undefined;
  }
  return measuredBytes(summary.size_bytes);
}

function temporarySummaryValue(t: Translate, summary: StorageSystemTemporarySummary): string {
  if (summary.status === "unavailable") {
    return t(STORAGE_UNAVAILABLE_VALUE_KEY);
  }
  if (summary.status === "not_applicable") {
    return t("system:storageNotApplicableValue");
  }
  const sizeBytes = temporarySummaryBytes(summary);
  if (sizeBytes === undefined) {
    return t(STORAGE_UNAVAILABLE_VALUE_KEY);
  }
  return formatGigabytes(sizeBytes);
}

function temporarySummaryDetail(
  t: Translate,
  status: StorageSystemTemporarySummary["status"],
): string {
  switch (status) {
    case "partial":
      return t("system:storageSystemTemporaryPartialDetail");
    case "unavailable":
      return t("system:storageSystemTemporaryUnavailableDetail");
    case "not_applicable":
      return t("system:storageSystemTemporaryNotApplicableDetail");
    case "measured":
      return t("system:storageSystemTemporaryCompleteDetail");
  }
}

function normalizeTemporaryWarnings(warnings: string[]): {
  values: string[];
  includesDeadline: boolean;
} {
  const values: string[] = [];
  let includesDeadline = false;
  for (const warning of warnings) {
    for (const line of warning.split(/\r?\n/)) {
      const value = line.trim();
      if (!value) continue;
      if (value === LEGACY_DEADLINE_WARNING) {
        includesDeadline = true;
        continue;
      }
      if (values.includes(value) || values.length >= TEMPORARY_WARNING_LIMIT) continue;
      values.push(value);
    }
  }
  return { values, includesDeadline };
}

function systemTemporaryResource(
  t: Translate,
  summary: StorageSystemTemporarySummary,
  temporaryArtifacts?: StorageTemporaryArtifactsSummary | null,
): StorageResource {
  const normalizedWarnings = normalizeTemporaryWarnings([
    ...(summary.warnings ?? []),
    ...summary.roots.flatMap((root) => root.warnings ?? []),
  ]);
  const includesDeadline =
    summary.reason === "deadline" ||
    summary.roots.some((root) => root.reason === "deadline") ||
    normalizedWarnings.includesDeadline;
  const value = temporarySummaryValue(t, summary);
  const statusDetail = temporarySummaryDetail(t, summary.status);
  const detailLines = summary.roots.map((root) => temporaryRootLine(t, root));
  if (includesDeadline) {
    detailLines.push(t("system:storageSystemTemporaryDeadlineDetail"));
  }
  return {
    id: SYSTEM_TEMPORARY_RESOURCE_ID,
    label: t("system:storageSystemTemporaryFolders"),
    value,
    detail: [t("system:storageSystemTemporaryInformational"), statusDetail].join(" "),
    detailLines,
    warning: normalizedWarnings.values.join(" · ") || undefined,
    source: "system_temporary",
    sizeBytes: temporarySummaryBytes(summary),
    partial: summary.status === "partial",
    systemTemporary: summary,
    temporaryArtifacts,
  };
}

export function systemTemporaryResourceOrPending(
  t: Translate,
  summary: StorageSummaryPartial["system_temporary"],
  progress?: StorageSourceProgress,
  temporaryArtifacts?: StorageSummaryPartial["temporary_artifacts"],
): StorageResource {
  if (!summary) {
    return pendingStorageResource(
      t,
      SYSTEM_TEMPORARY_RESOURCE_ID,
      t("system:storageSystemTemporaryFolders"),
      progress,
      "system_temporary",
    );
  }
  return systemTemporaryResource(t, summary, temporaryArtifacts);
}
