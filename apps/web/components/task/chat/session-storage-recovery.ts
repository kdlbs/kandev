import type { useTranslation } from "react-i18next";
import type { SessionErrorDetailsField } from "@/lib/session-error-details";
import { storageCauseFromError } from "@/lib/session-delivery-storage";
import { formatBytes } from "@/lib/utils/format-bytes";
import { formatDateTime } from "@/lib/i18n/formats";

type StorageTranslate = ReturnType<typeof useTranslation>["t"];

export type StorageRecoveryCopy = {
  title: string;
  summary: string;
  showSummary: boolean;
};

const STORAGE_TITLE_KEYS: Record<string, string> = {
  disk_space_low: "task:storageRecoveryDiskTitle",
  filesystem_full: "task:storageRecoveryFilesystemTitle",
  disk_quota_exceeded: "task:storageRecoveryQuotaTitle",
  journal_limit_reached: "task:storageRecoveryLimitTitle",
  stream_limit_reached: "task:storageRecoveryStreamLimitTitle",
  event_too_large: "task:storageRecoveryOversizedTitle",
  storage_write_failed: "task:storageRecoveryGenericTitle",
};

const STORAGE_BODY_KEYS: Record<string, { local: string; remote: string; unknown: string }> = {
  disk_space_low: {
    local: "task:storageRecoveryDiskBodyLocal",
    remote: "task:storageRecoveryDiskBodyRemote",
    unknown: "task:storageRecoveryDiskBodyUnknown",
  },
  filesystem_full: {
    local: "task:storageRecoveryFilesystemBodyLocal",
    remote: "task:storageRecoveryFilesystemBodyRemote",
    unknown: "task:storageRecoveryFilesystemBodyUnknown",
  },
  disk_quota_exceeded: {
    local: "task:storageRecoveryQuotaBodyLocal",
    remote: "task:storageRecoveryQuotaBodyRemote",
    unknown: "task:storageRecoveryQuotaBodyUnknown",
  },
};

const STORAGE_KIND_BODY_KEYS: Record<string, string> = {
  journal_limit_reached: "task:storageRecoveryJournalBody",
  stream_limit_reached: "task:storageRecoveryStreamBody",
  event_too_large: "task:storageRecoveryOversizedBody",
  storage_write_failed: "task:storageRecoveryGenericBody",
};

const STORAGE_REASON_KEYS: Record<string, string> = {
  disk_space_low: "task:storageReasonDiskSpaceLow",
  filesystem_full: "task:storageReasonFilesystemFull",
  disk_quota_exceeded: "task:storageReasonQuota",
  journal_limit_reached: "task:storageReasonJournalLimit",
  stream_limit_reached: "task:storageReasonStreamLimit",
  event_too_large: "task:storageReasonOversized",
  storage_write_failed: "task:storageReasonWriteFailed",
};

function storageBodyKey(reason: string, location: string): string {
  const bodies = STORAGE_BODY_KEYS[reason];
  if (bodies) {
    if (location === "local_runtime") return bodies.local;
    if (location === "executor_runtime") return bodies.remote;
    return bodies.unknown;
  }
  return STORAGE_KIND_BODY_KEYS[reason] ?? "task:storageRecoveryGenericBody";
}

function storageMeasurementCopy(
  cause: NonNullable<ReturnType<typeof storageCauseFromError>>,
  t: StorageTranslate,
): string | null {
  if (cause.diskAvailableBytes !== undefined && cause.diskRequiredBytes !== undefined) {
    return t("task:storageRecoveryAtFailure", {
      available: formatBytes(cause.diskAvailableBytes),
      required: formatBytes(cause.diskRequiredBytes),
    });
  }
  if (cause.retainedBytes !== undefined && cause.limitBytes !== undefined) {
    return t("task:storageRecoveryAtLimit", {
      retained: formatBytes(cause.retainedBytes),
      limit: formatBytes(cause.limitBytes),
    });
  }
  if (cause.eventBytes !== undefined) {
    return t("task:storageRecoveryAtEventSize", { size: formatBytes(cause.eventBytes) });
  }
  return null;
}

export function storageFailureCopy(
  cause: NonNullable<ReturnType<typeof storageCauseFromError>>,
  error: { occurredAt?: string } | null | undefined,
  t: StorageTranslate,
): StorageRecoveryCopy {
  const executor = cause.executorLabel || t("task:storageAdvisoryExecutorFallback");
  const parts = [t(storageBodyKey(cause.reason, cause.location), { executor })];
  const measurements = storageMeasurementCopy(cause, t);
  if (measurements) parts.push(measurements);
  const occurredAt = error?.occurredAt ? new Date(error.occurredAt) : null;
  if (occurredAt && !Number.isNaN(occurredAt.getTime())) {
    parts.push(t("task:storageRecoveryReported", { time: formatDateTime(occurredAt) }));
  }
  return {
    title: t(STORAGE_TITLE_KEYS[cause.reason] ?? "task:storageRecoveryGenericTitle"),
    summary: parts.join(" "),
    showSummary: true,
  };
}

/** Validated storage fields for the shared technical/copy disclosure. */
export function storageRecoveryDetailFields(
  cause: NonNullable<ReturnType<typeof storageCauseFromError>> | null,
  t: StorageTranslate,
): SessionErrorDetailsField[] {
  if (!cause) return [];
  const fields: SessionErrorDetailsField[] = [
    {
      label: t("task:storageDetailCause"),
      value: t(STORAGE_REASON_KEYS[cause.reason] ?? "task:storageReasonUnknown"),
    },
  ];
  const add = (label: string, value: string | undefined) => {
    if (value) fields.push({ label, value });
  };
  if (cause.diskAvailableBytes !== undefined) {
    add(t("task:storageDetailAvailable"), formatBytes(cause.diskAvailableBytes));
  }
  if (cause.diskRequiredBytes !== undefined) {
    add(t("task:storageDetailRequired"), formatBytes(cause.diskRequiredBytes));
  }
  if (cause.retainedBytes !== undefined) {
    add(t("task:storageDetailRetained"), formatBytes(cause.retainedBytes));
  }
  if (cause.limitBytes !== undefined) {
    add(t("task:storageDetailLimit"), formatBytes(cause.limitBytes));
  }
  if (cause.eventBytes !== undefined) {
    add(t("task:storageDetailEventSize"), formatBytes(cause.eventBytes));
  }
  if (cause.observedAt) {
    const observedAt = new Date(cause.observedAt);
    if (!Number.isNaN(observedAt.getTime())) {
      add(t("task:storageDetailObservedAt"), formatDateTime(observedAt));
    }
  }
  if (cause.executorLabel) add(t("task:storageDetailExecutor"), cause.executorLabel);
  return fields;
}
