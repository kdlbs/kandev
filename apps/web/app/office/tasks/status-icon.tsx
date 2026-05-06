"use client";

import { cn } from "@/lib/utils";
import type { OfficeTaskStatus } from "@/lib/state/slices/office/types";
import { normalizeTaskStatus } from "./normalize-status";

/**
 * Status icon for office tasks. Colored outline ring per status, with an
 * inner dot for `done`; `cancelled` gets a muted ring (no diagonal slash).
 */
const statusStyles: Record<OfficeTaskStatus, string> = {
  backlog: "border-muted-foreground/60 text-muted-foreground/60",
  todo: "border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400",
  in_progress: "border-yellow-600 text-yellow-600 dark:border-yellow-400 dark:text-yellow-400",
  in_review: "border-violet-600 text-violet-600 dark:border-violet-400 dark:text-violet-400",
  blocked: "border-red-600 text-red-600 dark:border-red-400 dark:text-red-400",
  done: "border-green-600 text-green-600 dark:border-green-400 dark:text-green-400",
  cancelled: "border-neutral-500 text-neutral-500",
};

const statusLabels: Record<OfficeTaskStatus, string> = {
  backlog: "Backlog",
  todo: "Todo",
  in_progress: "In Progress",
  in_review: "In Review",
  blocked: "Blocked",
  done: "Done",
  cancelled: "Cancelled",
};

type StatusIconProps = {
  /** Accepts any backend / display status string; gets normalised internally. */
  status: string;
  className?: string;
};

export function StatusIcon({ status, className }: StatusIconProps) {
  const normalised = normalizeTaskStatus(status);
  const label = statusLabels[normalised];
  return (
    <span
      className={cn(
        "relative inline-flex h-4 w-4 rounded-full border-2 shrink-0",
        statusStyles[normalised],
        className,
      )}
      aria-label={label}
      title={label}
    >
      {normalised === "done" && (
        <span className="absolute inset-0 m-auto h-1.5 w-1.5 rounded-full bg-current" />
      )}
    </span>
  );
}
