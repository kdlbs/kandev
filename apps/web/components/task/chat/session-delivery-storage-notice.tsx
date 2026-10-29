"use client";

import { useEffect, useRef, useState } from "react";
import Link from "@/components/routing/app-link";
import { IconAlertTriangle, IconChevronDown } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type { RuntimeStorageHealth } from "@/lib/types/http";
import {
  formatStorageUsage,
  isRuntimeStorageStale,
  runtimeStorageUsagePercent,
  storageAdvisoryVisible,
} from "@/lib/session-delivery-storage";
import { formatBytes } from "@/lib/utils/format-bytes";
import { formatDateTime } from "@/lib/i18n/formats";

type StorageTranslate = ReturnType<typeof useTranslation>["t"];

type NoticeView = {
  usage: string | null;
  remote: boolean;
  executor: string;
  pressure: boolean;
  observedAt: Date | null;
};

const PRESSURE_REASON_KEYS: Record<string, string> = {
  disk_space_low: "task:storagePressureLowSpace",
  filesystem_full: "task:storagePressureFilesystemFull",
  disk_quota_exceeded: "task:storagePressureQuota",
  journal_limit_reached: "task:storagePressureJournalLimit",
  stream_limit_reached: "task:storagePressureStreamLimit",
  event_too_large: "task:storagePressureOversized",
  storage_write_failed: "task:storagePressureWriteFailed",
};

function pressureReasonLabel(health: RuntimeStorageHealth, t: StorageTranslate): string {
  const reason = health.failure_storage?.reason;
  const key = reason ? PRESSURE_REASON_KEYS[reason] : undefined;
  return key ? t(key) : t("task:storagePressureGeneric");
}

function buildNoticeView(health: RuntimeStorageHealth, t: StorageTranslate): NoticeView {
  const usageNumber = formatStorageUsage(runtimeStorageUsagePercent(health));
  const remote = health.location === "executor_runtime";
  const parsed = health.observed_at ? new Date(health.observed_at) : null;
  return {
    usage: usageNumber,
    remote,
    executor: health.executor_label?.trim() || t("task:storageAdvisoryExecutorFallback"),
    pressure: health.write_pressure === true,
    observedAt: parsed && !Number.isNaN(parsed.getTime()) ? parsed : null,
  };
}

/** Local monotonic elapsed time since this revision entered the browser. */
function useReceiptElapsed(key: string): number {
  const receipt = useRef({ key, at: performance.now() });
  if (receipt.current.key !== key) receipt.current = { key, at: performance.now() };
  const [, setTick] = useState(0);
  useEffect(() => {
    const timer = setInterval(() => setTick((tick) => tick + 1), 10_000);
    return () => clearInterval(timer);
  }, [key]);
  return Math.max(0, performance.now() - receipt.current.at);
}

function NoticeGuidance({ health, view }: { health: RuntimeStorageHealth; view: NoticeView }) {
  const { t } = useTranslation();
  if (view.pressure) {
    const reason = health.failure_storage?.reason;
    if (reason === "event_too_large") {
      return <>{t("task:storageAdvisoryPressureGuidanceOversized")}</>;
    }
    if (reason === "journal_limit_reached" || reason === "stream_limit_reached") {
      return <>{t("task:storageAdvisoryPressureGuidanceLimit")}</>;
    }
    if (reason === "storage_write_failed") {
      return <>{t("task:storageAdvisoryPressureGuidanceWriteFailed")}</>;
    }
    return <>{t("task:storageAdvisoryPressureGuidance", { executor: view.executor })}</>;
  }
  if (view.remote) {
    return <>{t("task:storageAdvisoryGuidanceRemote", { executor: view.executor })}</>;
  }
  return <>{t("task:storageAdvisoryGuidanceLocal")}</>;
}

function NoticeDetails({ health, view }: { health: RuntimeStorageHealth; view: NoticeView }) {
  const { t } = useTranslation();
  const kind =
    health.storage_kind === "workspace"
      ? t("task:storageAdvisoryWorkspaceKind")
      : t("task:storageAdvisoryJournalKind");
  const free =
    health.available_bytes !== undefined
      ? t("task:storageAdvisoryFree", { free: formatBytes(health.available_bytes) })
      : null;
  const total =
    health.total_bytes !== undefined
      ? t("task:storageAdvisoryTotal", { total: formatBytes(health.total_bytes) })
      : null;
  const reason = health.failure_storage?.reason;
  const reasonKey = reason ? PRESSURE_REASON_KEYS[reason] : undefined;
  return (
    <div
      data-testid="session-delivery-storage-details-panel"
      className="text-muted-foreground min-w-0 space-y-1 text-xs"
    >
      <p className="min-w-0 break-words">
        <NoticeGuidance health={health} view={view} />
      </p>
      {reasonKey && (
        <p className="min-w-0 break-words">
          {t("task:storageAdvisoryPressureCause", { cause: t(reasonKey) })}
        </p>
      )}
      <p className="min-w-0 break-words tabular-nums">
        {[kind, free, total].filter(Boolean).join(" · ")}
      </p>
      {view.remote && health.executor_label && (
        <p className="min-w-0 break-words">
          {t("task:storageAdvisoryExecutorLabel", { executor: health.executor_label })}
        </p>
      )}
      {view.observedAt && (
        <p className="min-w-0 break-words">
          {t("task:storageAdvisoryObserved", { time: formatDateTime(view.observedAt) })}
        </p>
      )}
    </div>
  );
}

/** One compact advisory or blocked-output row. It never offers Resume. */
export function SessionDeliveryStorageNotice({ health }: { health?: RuntimeStorageHealth | null }) {
  const { t } = useTranslation();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const visible = storageAdvisoryVisible(health);
  const receiptKey = health
    ? `${health.epoch}:${health.revision}:${health.write_pressure ? 1 : 0}`
    : "";
  const elapsed = useReceiptElapsed(receiptKey);
  if (!health || !visible) return null;
  const view = buildNoticeView(health, t);
  const stale = isRuntimeStorageStale(health.age_seconds, elapsed);
  const usageLabel = view.usage ? t("task:storageAdvisoryUsage", { usage: view.usage }) : null;
  const primaryLabel = view.pressure ? pressureReasonLabel(health, t) : usageLabel;
  if (!primaryLabel) return null;
  return (
    <div
      data-testid="session-delivery-storage-notice"
      data-storage-state={health.state}
      data-pressure={view.pressure ? "true" : "false"}
      data-stale={stale ? "true" : "false"}
      className="border-border/60 bg-amber-500/5 my-1 flex flex-col gap-1 rounded-md border px-2 py-2"
    >
      <StorageNoticeRow
        health={health}
        view={view}
        primaryLabel={primaryLabel}
        stale={stale}
        detailsOpen={detailsOpen}
        onToggleDetails={() => setDetailsOpen((open) => !open)}
      />
    </div>
  );
}

function StorageNoticeRow({
  health,
  view,
  primaryLabel,
  stale,
  detailsOpen,
  onToggleDetails,
}: {
  health: RuntimeStorageHealth;
  view: NoticeView;
  primaryLabel: string;
  stale: boolean;
  detailsOpen: boolean;
  onToggleDetails: () => void;
}) {
  const { t } = useTranslation();
  const usageLabel = view.usage ? t("task:storageAdvisoryUsage", { usage: view.usage }) : null;
  return (
    <>
      <div className="flex min-w-0 items-center gap-2">
        <IconAlertTriangle className="size-4 shrink-0 text-amber-600" aria-hidden="true" />
        <span className="flex min-w-0 flex-1 items-baseline gap-1 overflow-hidden text-xs">
          {stale && (
            <span className="shrink-0 font-medium">{t("task:storageAdvisoryLastReported")}:</span>
          )}
          {view.remote && health.executor_label && (
            <>
              <span className="min-w-0 truncate" title={health.executor_label}>
                {t("task:storageAdvisoryRemotePrefix", { executor: health.executor_label })}
              </span>{" "}
            </>
          )}
          <span className="shrink-0 truncate font-medium">{primaryLabel}</span>
          {view.pressure && usageLabel && (
            <span className="text-muted-foreground hidden shrink-0 tabular-nums md:inline">
              {usageLabel}
            </span>
          )}
          {!view.pressure && health.available_bytes !== undefined && (
            <span className="text-muted-foreground hidden shrink-0 tabular-nums md:inline">
              {t("task:storageAdvisoryFree", { free: formatBytes(health.available_bytes) })}
            </span>
          )}
        </span>
        <span className="ml-auto flex shrink-0 items-center gap-1">
          {!view.remote && (
            <Link
              href="/settings/system/storage"
              data-testid="session-delivery-storage-settings"
              className={controlSizingClassName(
                "standard",
                "inline-flex cursor-pointer items-center rounded px-2 text-xs font-medium underline-offset-2 hover:bg-muted hover:underline focus-visible:ring-2 focus-visible:ring-ring",
              )}
            >
              {t("task:storageAdvisorySettings")}
            </Link>
          )}
          <button
            type="button"
            aria-expanded={detailsOpen}
            data-testid="session-delivery-storage-details"
            onClick={onToggleDetails}
            className={controlSizingClassName(
              "standard",
              "inline-flex cursor-pointer items-center gap-1 rounded px-2 text-xs hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
            )}
          >
            {t("task:storageAdvisoryDetails")}
            <IconChevronDown
              className={`size-3 transition-transform ${detailsOpen ? "rotate-180" : ""}`}
              aria-hidden="true"
            />
          </button>
        </span>
      </div>
      {detailsOpen && <NoticeDetails health={health} view={view} />}
    </>
  );
}
