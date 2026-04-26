"use client";

import { cn } from "@/lib/utils";
import type { OrchestrateIssueStatus } from "@/lib/state/slices/orchestrate/types";

const statusStyles: Record<OrchestrateIssueStatus, string> = {
  backlog: "border-muted-foreground text-muted-foreground",
  todo: "border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400",
  in_progress: "border-yellow-600 text-yellow-600 dark:border-yellow-400 dark:text-yellow-400",
  in_review: "border-violet-600 text-violet-600 dark:border-violet-400 dark:text-violet-400",
  blocked: "border-red-600 text-red-600 dark:border-red-400 dark:text-red-400",
  done: "border-green-600 bg-green-600 text-green-600 dark:border-green-400 dark:bg-green-400 dark:text-green-400",
  cancelled: "border-neutral-500 text-neutral-500",
};

type StatusIconProps = {
  status: OrchestrateIssueStatus;
  className?: string;
};

export function StatusIcon({ status, className }: StatusIconProps) {
  const isFilled = status === "done";
  const isCancelled = status === "cancelled";

  return (
    <span
      className={cn(
        "inline-block shrink-0 rounded-full border-2",
        statusStyles[status],
        isFilled && "border-0",
        className,
      )}
      style={{ width: "1em", height: "1em", position: "relative" }}
      aria-label={status.replace("_", " ")}
    >
      {isCancelled && (
        <span className="absolute inset-0 flex items-center justify-center">
          <span className="block h-0.5 w-3/4 bg-current rounded-full rotate-[-45deg]" />
        </span>
      )}
      {status === "blocked" && (
        <span className="absolute bottom-0 right-0 block h-1 w-1 rounded-full bg-current" />
      )}
    </span>
  );
}
