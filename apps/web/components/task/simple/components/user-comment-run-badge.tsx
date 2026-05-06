"use client";

import { IconAlertCircle, IconLoader2 } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { TaskRunStatus } from "@/app/office/tasks/[id]/types";

type UserCommentRunBadgeProps = {
  status: TaskRunStatus;
  errorMessage?: string;
};

const BASE_PILL = "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium";

/**
 * Renders the lifecycle of the wakeup queued by a user comment.
 *
 *  - queued    → muted "Queued" pill with a small dot.
 *  - claimed   → primary-tint "Working…" pill with a spinner.
 *  - finished  → null (the agent reply lands moments later via the
 *                existing comment WS path; the badge naturally hides).
 *  - failed    → red "Failed" pill; the optional `errorMessage` is
 *                surfaced via tooltip.
 *  - cancelled → muted "Cancelled" pill.
 *
 * The component carries `data-testid="user-comment-run-badge"` and a
 * `data-status` attribute so e2e tests can pin the rendered state.
 */
export function UserCommentRunBadge({ status, errorMessage }: UserCommentRunBadgeProps) {
  if (status === "finished") return null;

  if (status === "queued") {
    return (
      <span
        className={`${BASE_PILL} bg-muted text-muted-foreground`}
        data-testid="user-comment-run-badge"
        data-status={status}
      >
        <span
          className="inline-block h-1.5 w-1.5 rounded-full bg-muted-foreground/60"
          aria-hidden
        />
        Queued
      </span>
    );
  }

  if (status === "claimed") {
    return (
      <span
        className={`${BASE_PILL} bg-primary/10 text-primary border border-primary/20`}
        data-testid="user-comment-run-badge"
        data-status={status}
      >
        <IconLoader2 className="h-3 w-3 animate-spin" aria-hidden />
        Working…
      </span>
    );
  }

  if (status === "cancelled") {
    return (
      <span
        className={`${BASE_PILL} bg-muted text-muted-foreground`}
        data-testid="user-comment-run-badge"
        data-status={status}
      >
        Cancelled
      </span>
    );
  }

  // failed
  const failedPill = (
    <span
      className={`${BASE_PILL} bg-red-500/10 text-red-600 border border-red-500/20`}
      data-testid="user-comment-run-badge"
      data-status={status}
    >
      <IconAlertCircle className="h-3 w-3" aria-hidden />
      Failed
    </span>
  );

  if (!errorMessage) return failedPill;

  return (
    <Tooltip>
      <TooltipTrigger asChild>{failedPill}</TooltipTrigger>
      <TooltipContent data-testid="user-comment-run-badge-tooltip">{errorMessage}</TooltipContent>
    </Tooltip>
  );
}
