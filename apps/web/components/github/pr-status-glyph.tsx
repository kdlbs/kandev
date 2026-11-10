"use client";

import {
  IconAlertTriangleFilled,
  IconGitPullRequest,
  IconLockFilled,
  IconCircleXFilled,
} from "@tabler/icons-react";
import type { PRCIState } from "@/lib/types/github";
import { cn } from "@/lib/utils";

const AMBER = "text-[#D97706] dark:text-[#FBBF24]";

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

const CI_BUBBLE_CLASS =
  "absolute -right-1 -top-1 inline-flex h-2.5 w-2.5 items-center justify-center rounded-full bg-background";

function PRCIBubble({ state }: { state: PRCIState }) {
  if (state === "none") return null;
  if (state === "failed") {
    return (
      <span data-testid="pr-ci-bubble-failed" className={cn(CI_BUBBLE_CLASS, "text-red-500")}>
        <IconCircleXFilled className="h-2 w-2" />
      </span>
    );
  }
  if (state === "approval_required") {
    return (
      <span data-testid="pr-ci-bubble-approval-required" className={cn(CI_BUBBLE_CLASS, AMBER)}>
        <IconLockFilled className="h-2 w-2" />
      </span>
    );
  }
  if (state === "attention") {
    return (
      <span data-testid="pr-ci-bubble-attention" className={cn(CI_BUBBLE_CLASS, AMBER)}>
        <IconAlertTriangleFilled className="h-2 w-2" />
      </span>
    );
  }
  return (
    <span data-testid="pr-ci-bubble-in-progress" className={CI_BUBBLE_CLASS}>
      <span className="h-2 w-2 rounded-full border-[1.5px] border-[#D97706] border-t-transparent motion-safe:animate-spin dark:border-[#FBBF24]" />
    </span>
  );
}

export function PRStatusGlyph({
  colorClassName,
  hasMergeConflicts = false,
  ciState = "none",
  autoFixEnabled = false,
  autoMergeEnabled = false,
  size = "task",
}: {
  colorClassName?: string;
  hasMergeConflicts?: boolean;
  ciState?: PRCIState;
  autoFixEnabled?: boolean;
  autoMergeEnabled?: boolean;
  size?: "task" | "topbar";
}) {
  // The topbar button shrinks SVGs without an explicit size-* class.
  const dimension = size === "topbar" ? "size-4" : "h-3.5 w-3.5";
  return (
    <span className="inline-flex shrink-0 items-center gap-1" aria-hidden="true">
      <span className={cn("relative inline-flex shrink-0", dimension, colorClassName)}>
        <IconGitPullRequest className={cn(dimension, colorClassName)} />
        <AutomationIndicatorDots
          autoFixEnabled={autoFixEnabled}
          autoMergeEnabled={autoMergeEnabled}
        />
        <PRCIBubble state={ciState} />
      </span>
      {hasMergeConflicts && (
        <span
          data-testid="pr-merge-conflict-warning"
          className="inline-flex h-3 w-3 shrink-0 items-center justify-center text-red-500"
        >
          <IconAlertTriangleFilled className="h-3 w-3" />
        </span>
      )}
    </span>
  );
}
