"use client";

import { IconAlertTriangleFilled, IconGitPullRequest } from "@tabler/icons-react";
import { cn } from "@/lib/utils";

export function AutomationIndicatorDots({
  autoFixEnabled,
  autoMergeEnabled,
}: {
  autoFixEnabled: boolean;
  autoMergeEnabled: boolean;
}) {
  return (
    <>
      {autoFixEnabled && (
        <span
          data-testid="pr-task-automation-auto-fix"
          className="absolute left-0 top-0 h-1.5 w-1.5 rounded-full bg-yellow-400 ring-1 ring-background"
          aria-hidden="true"
        />
      )}
      {autoMergeEnabled && (
        <span
          data-testid="pr-task-automation-auto-merge"
          className="absolute right-0 bottom-0 h-1.5 w-1.5 rounded-full bg-purple-500 ring-1 ring-background"
          aria-hidden="true"
        />
      )}
    </>
  );
}

export function PRStatusGlyph({
  colorClassName,
  hasMergeConflicts = false,
  autoFixEnabled = false,
  autoMergeEnabled = false,
  size = "task",
}: {
  colorClassName?: string;
  hasMergeConflicts?: boolean;
  autoFixEnabled?: boolean;
  autoMergeEnabled?: boolean;
  size?: "task" | "topbar";
}) {
  const dimension = size === "topbar" ? "h-4 w-4" : "h-3.5 w-3.5";
  return (
    <span
      className={cn("relative inline-flex shrink-0", dimension, colorClassName)}
      aria-hidden="true"
    >
      <IconGitPullRequest className={cn(dimension, colorClassName)} />
      <AutomationIndicatorDots
        autoFixEnabled={autoFixEnabled}
        autoMergeEnabled={autoMergeEnabled}
      />
      {/* Reserve top-right for conflict warnings; auto-merge dots occupy lower-right. */}
      {hasMergeConflicts && (
        <span
          data-testid="pr-merge-conflict-warning"
          className="absolute -right-1 -top-1 inline-flex h-2.5 w-2.5 items-center justify-center rounded-full bg-background text-red-500"
        >
          <IconAlertTriangleFilled className="h-2 w-2" />
        </span>
      )}
    </span>
  );
}
