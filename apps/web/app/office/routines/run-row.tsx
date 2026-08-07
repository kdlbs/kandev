"use client";

import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import type { RoutineRun } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

const FALLBACK_COLORS: Record<string, string> = {
  received: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300",
  task_created: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
  skipped: "bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400",
  coalesced: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300",
  failed: "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300",
  done: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
  cancelled: "bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400",
};

function formatTime(dateStr?: string): string {
  if (!dateStr) return "--";
  return new Date(dateStr).toLocaleString();
}

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// record keys are the wire run-status values. This used to render
// `status.replace(/_/g, " ")`, which showed the identifier as pseudo-English.
const FALLBACK_STATUS_LABEL_KEYS: Record<string, string> = {
  received: "office:runStatusReceived",
  task_created: "office:runStatusTaskCreated",
  skipped: "office:runStatusSkipped",
  coalesced: "office:runStatusCoalesced",
  failed: "office:runStatusFailed",
  done: "office:runStatusDone",
  cancelled: "office:runStatusCancelled",
};

type RunRowProps = {
  run: RoutineRun;
};

export function RunRow({ run }: RunRowProps) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  const metaStatus = meta?.routineRunStatuses.find((s) => s.id === run.status);
  const colorClass = metaStatus?.color ?? FALLBACK_COLORS[run.status] ?? "";
  const fallbackKey = FALLBACK_STATUS_LABEL_KEYS[run.status];
  // `?? run.status` keeps an unknown wire value visible rather than blank.
  const label = metaStatus?.label ?? (fallbackKey ? t(fallbackKey) : run.status);

  return (
    <div className="flex items-center gap-3 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors">
      <Badge className={colorClass}>{label}</Badge>
      <span className="text-xs text-muted-foreground capitalize">{run.source}</span>
      <span className="flex-1 text-xs text-muted-foreground font-mono truncate">
        {run.dispatchFingerprint ? run.dispatchFingerprint.slice(0, 12) : "--"}
      </span>
      {run.linkedTaskId && (
        <span className="text-xs text-blue-600 dark:text-blue-400 font-mono">
          {run.linkedTaskId.slice(0, 12)}
        </span>
      )}
      <span className="text-xs text-muted-foreground shrink-0">{formatTime(run.createdAt)}</span>
    </div>
  );
}
