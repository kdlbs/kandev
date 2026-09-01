"use client";

import {
  IconAlertTriangle,
  IconCheck,
  IconCircleDashed,
  IconClockHour4,
  IconLoader2,
  type Icon as TablerIcon,
} from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import type { RemoteExecutorStatusData } from "@/hooks/domains/session/remote-executor-status-resource";
import { getExecutorLabel, getExecutorStatusIcon } from "@/lib/executor-icons";
import { cn, formatRelativeTime } from "@/lib/utils";

type StatusRow = {
  testId: string;
  label: string;
  value: string;
  Icon: TablerIcon;
  tone: string;
  technical?: boolean;
  tabular?: boolean;
};

const DEFAULT_TONE = "text-foreground";

function statusPresentation(status: RemoteExecutorStatusData | null, loading: boolean) {
  if (status?.remote_status_error) {
    return { Icon: IconAlertTriangle, tone: "text-red-600 dark:text-red-400" };
  }
  const state = status?.remote_state?.toLowerCase();
  if (state === "running" || state === "ready" || state === "connected") {
    return { Icon: IconCheck, tone: "text-emerald-600 dark:text-emerald-400" };
  }
  if (
    state === "pending" ||
    state === "starting" ||
    state === "provisioning" ||
    state === "waiting"
  ) {
    return { Icon: IconClockHour4, tone: "text-amber-600 dark:text-amber-400" };
  }
  if (loading) return { Icon: IconLoader2, tone: "text-muted-foreground" };
  return { Icon: IconCircleDashed, tone: "text-muted-foreground" };
}

function SummaryRow({ row }: { row: StatusRow }) {
  const { Icon } = row;
  return (
    <div
      data-testid={row.testId}
      className="grid grid-cols-[minmax(5.25rem,auto)_minmax(0,1fr)] items-start gap-x-3 text-xs"
    >
      <dt className="text-muted-foreground">{row.label}</dt>
      <dd className={cn("flex min-w-0 items-start gap-1.5 font-medium", row.tone)}>
        <Icon
          aria-hidden="true"
          className={cn("mt-px size-3.5 shrink-0", Icon === IconLoader2 && "animate-spin")}
        />
        <span
          className={cn(
            "min-w-0 break-words [overflow-wrap:anywhere]",
            row.technical && "font-mono",
            row.tabular && "tabular-nums",
          )}
        >
          {row.value}
        </span>
      </dd>
    </div>
  );
}

function statusRows(
  status: RemoteExecutorStatusData | null,
  loading: boolean,
  t: TFunction,
): StatusRow[] {
  const presentation = statusPresentation(status, loading);
  const rows: StatusRow[] = [
    {
      testId: "remote-executor-status-state",
      label: t("task:status"),
      value:
        status?.remote_state ??
        (loading ? t("task:loadingStatus") : t("task:executorEnvironmentIsUnavailable")),
      ...presentation,
    },
  ];
  if (status?.remote_restarts !== undefined) {
    rows.push({
      testId: "remote-executor-status-restarts",
      label: t("task:restarts"),
      value: String(status.remote_restarts),
      Icon: IconCircleDashed,
      tone: DEFAULT_TONE,
      tabular: true,
    });
  }
  if (status?.remote_workspace_kind) {
    rows.push({
      testId: "remote-executor-status-workspace",
      label: t("task:workspaceMode"),
      value: status.remote_workspace_kind,
      Icon: IconCircleDashed,
      tone: DEFAULT_TONE,
      technical: true,
    });
  }
  if (status?.remote_created_at) {
    rows.push({
      testId: "remote-executor-status-created",
      label: t("task:created"),
      value: formatRelativeTime(status.remote_created_at),
      Icon: IconClockHour4,
      tone: DEFAULT_TONE,
    });
  }
  if (status?.remote_checked_at) {
    rows.push({
      testId: "remote-executor-status-checked",
      label: t("task:remoteLastCheck"),
      value: formatRelativeTime(status.remote_checked_at),
      Icon: IconClockHour4,
      tone: DEFAULT_TONE,
    });
  }
  if (status?.remote_status_error) {
    rows.push({
      testId: "remote-executor-status-error",
      label: t("task:error"),
      value: status.remote_status_error,
      Icon: IconAlertTriangle,
      tone: "text-red-600 dark:text-red-400",
    });
  }
  return rows;
}

export function RemoteExecutorTaskStatusSummary({
  executorType,
  remoteName,
  status,
  loading,
}: {
  executorType?: string | null;
  remoteName: string;
  status: RemoteExecutorStatusData | null;
  loading: boolean;
}) {
  const { t } = useTranslation();
  const icon = getExecutorStatusIcon(executorType, Boolean(status?.remote_status_error));
  const Icon = icon.Icon;
  const rows = statusRows(status, loading, t);
  return (
    <section data-testid="remote-executor-status-summary" className="w-full">
      <div className="flex items-start gap-2">
        <Icon aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="text-[11px] font-medium text-muted-foreground">
            {getExecutorLabel(executorType ?? "")}
          </div>
          <div
            data-testid="remote-executor-status-identity"
            className={cn(
              "text-pretty break-words text-sm font-medium leading-snug text-foreground [overflow-wrap:anywhere]",
              executorType === "k8s" && "font-mono text-xs",
            )}
          >
            {remoteName}
          </div>
        </div>
        {loading ? (
          <IconLoader2
            aria-label={t("task:loadingStatus")}
            className="size-3.5 shrink-0 animate-spin text-muted-foreground"
          />
        ) : null}
      </div>
      <dl className="mt-2.5 space-y-1.5 pl-6">
        {rows.map((row) => (
          <SummaryRow key={row.testId} row={row} />
        ))}
      </dl>
    </section>
  );
}
