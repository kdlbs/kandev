"use client";

import { IconClockHour4 } from "@tabler/icons-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { useOptionalAppStore } from "@/components/state-provider";
import { formatRelativeTime } from "@/lib/utils";
import type { TaskStatusSummaryLaunchQueue } from "@/lib/types/task-status-summary";
import {
  buildLaunchQueueViewModel,
  type LaunchQueueViewModel,
} from "@/lib/tasks/launch-queue-view-model";

function reasonLabel(
  reason: LaunchQueueViewModel["reason"],
  retrying: boolean,
  t: (key: string) => string,
  all = false,
): string {
  switch (reason) {
    case "session_capacity":
      return t(
        all ? "task:launchQueueWaitingExecutionCapacity" : "task:launchQueueWaitingGlobalCapacity",
      );
    case "ownership_unavailable":
      return t("task:launchQueueOwnershipUnavailable");
    case "replay_error":
      return t(retrying ? "task:launchQueueReplayError" : "task:launchQueueReplayErrorStopped");
  }
}

function retryStatusLabel(view: LaunchQueueViewModel, t: (key: string) => string): string {
  if (!view.retrying) return t("task:launchQueueRetryStopped");
  if (view.reason === "session_capacity") return t("task:launchQueueAutomaticRetry");
  return t("task:launchQueueRetryPending");
}

function capacityLabel(
  view: LaunchQueueViewModel,
  isConnected: boolean,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  if (!view.capacity) {
    return t(
      view.retrying
        ? "task:launchQueueCapacityUnavailable"
        : "task:launchQueueCapacityUnavailableStopped",
    );
  }

  const all = view.capacity.policy === "all";
  let key = all ? "task:launchQueueExecutionCapacityStale" : "task:launchQueueCapacityStale";
  if (view.capacityFreshness === "current") {
    key = all ? "task:launchQueueExecutionCapacity" : "task:launchQueueCapacity";
  } else if (view.capacityFreshness === "stale" && !isConnected) {
    key = all
      ? "task:launchQueueExecutionCapacityDisconnected"
      : "task:launchQueueCapacityDisconnected";
  }
  return t(key, {
    inUse: view.capacity.inUse,
    limit: view.capacity.limit,
    checkedAt: formatRelativeTime(view.capacity.observedAt),
  });
}

export function LaunchQueueStatus({
  queue,
  isConnected,
}: {
  queue: TaskStatusSummaryLaunchQueue | null | undefined;
  isConnected?: boolean;
}) {
  const { t } = useTranslation();
  const connected = useOptionalAppStore((state) => state.connection.status === "connected", true);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!queue) return;
    const interval = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(interval);
  }, [queue]);

  const view = buildLaunchQueueViewModel(queue, {
    now,
    isConnected: isConnected ?? connected,
  });
  if (!view) return null;

  return <LaunchQueueStatusContent view={view} t={t} isConnected={isConnected ?? connected} />;
}

function LaunchQueueStatusContent({
  view,
  t,
  isConnected,
}: {
  view: LaunchQueueViewModel;
  t: (key: string, values?: Record<string, unknown>) => string;
  isConnected: boolean;
}) {
  const destinationLabel = useOptionalAppStore((state) => {
    if (!view.destinationId) return "";
    return (
      state.agentProfiles.items.find((profile) => profile.id === view.destinationId)?.label ?? ""
    );
  }, "");

  const destination = destinationLabel || t("task:launchQueueUnknownDestination");
  const retryLabel = retryStatusLabel(view, t);

  return (
    <section
      data-testid="task-launch-queue-status"
      className="min-w-0 shrink-0 border-b border-border/70 bg-muted/20 px-3 py-2"
    >
      <span
        data-testid="task-launch-queue-live-status"
        role="status"
        aria-live="polite"
        className="sr-only"
      >
        {t("task:launchQueueIndicator")}
      </span>
      <div className="flex min-w-0 items-start gap-2">
        <IconClockHour4
          className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1 space-y-0.5 text-xs">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
            <span className="font-medium text-foreground">{t("task:launchQueueTitle")}</span>
            <span className="rounded bg-amber-500/15 px-1.5 py-0.5 font-medium text-amber-700 dark:text-amber-300">
              {t("task:launchQueueLabel")}
            </span>
          </div>
          <p className="min-w-0 break-words text-foreground">
            {t("task:launchQueueDestination", { destination })}
          </p>
          <p className="min-w-0 break-words text-muted-foreground">
            {reasonLabel(view.reason, view.retrying, t, view.capacity?.policy === "all")}
          </p>
          {view.reason === "session_capacity" && (
            <>
              <p className="min-w-0 break-words text-muted-foreground">
                {t(
                  view.capacity?.policy === "all"
                    ? "task:launchQueueExecutionScope"
                    : "task:launchQueueGlobalScope",
                )}
              </p>
              <p className="min-w-0 break-words text-muted-foreground">
                {t("task:launchQueueGlobalScopeHelp")}
              </p>
              <Link
                href="/settings/preferences/task-behavior#setting-session-capacity"
                data-testid="launch-queue-session-capacity-link"
                className={settingsActionClassName(
                  "mt-1 inline-flex max-w-full whitespace-normal text-left leading-tight",
                )}
              >
                {t(
                  view.capacity?.policy === "all"
                    ? "task:launchQueueConfigureExecutionCapacity"
                    : "task:launchQueueConfigureCapacity",
                )}
              </Link>
            </>
          )}
          <p className="min-w-0 break-words tabular-nums text-muted-foreground">
            {capacityLabel(view, isConnected, t)}
          </p>
          <LaunchQueueGradeCapacity view={view} t={t} />
          <p className="min-w-0 break-words text-muted-foreground">
            {t("task:launchQueueSince", { time: formatRelativeTime(view.queuedAt) })}
          </p>
          <p className="min-w-0 break-words text-muted-foreground">{retryLabel}</p>
        </div>
      </div>
    </section>
  );
}

function LaunchQueueGradeCapacity({
  view,
  t,
}: {
  view: LaunchQueueViewModel;
  t: (key: string, values?: Record<string, unknown>) => string;
}) {
  const capacity = view.capacity;
  if (capacity?.policy !== "all") return null;
  const unknown = t("common:unknown");
  return (
    <div className="space-y-1">
      <p className="font-medium">{t("task:launchQueueGradeHeading")}</p>
      {capacity.grades?.length ? (
        <dl className="space-y-1">
          {capacity.grades.map((row) => (
            <div key={row.grade} className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-x-3">
              <dt className="min-w-0 break-words">{row.grade}</dt>
              <dd className="text-right tabular-nums">
                {t("task:launchQueueGradeUsage", {
                  inUse: row.inUse ?? unknown,
                  limit: row.limit ?? unknown,
                })}
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <p className="text-muted-foreground">{t("task:launchQueueGradesUnavailable")}</p>
      )}
      {capacity.needsConfiguration && (
        <p className="text-amber-700 dark:text-amber-300">
          {t("task:launchQueueGradeNeedsConfiguration")}
        </p>
      )}
    </div>
  );
}
