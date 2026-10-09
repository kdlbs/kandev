"use client";

import type { ReactElement } from "react";
import Link from "@/components/routing/app-link";
import { IconRobot } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import {
  useSenderTaskBadgeModel,
  type SenderTaskInfo,
} from "@/hooks/domains/session/use-sender-task-badge-model";
import { linkToTask } from "@/lib/links";
import { Trans } from "react-i18next";

type SenderTaskTooltipProps = {
  sessionName: string;
  fullTitle: string;
};

type SenderTaskBadgeProps = {
  sender: SenderTaskInfo;
  /** Task that owns the message or queue row containing this badge. */
  destinationTaskId: string;
  /** Optional override for the badge size — defaults to "sm" (chat bubbles). */
  size?: "xs" | "sm";
};

function wrapBadgeWithTaskLink(
  liveTask: { id: string } | null | undefined,
  sender: SenderTaskInfo,
  linkAriaLabel: string,
  inner: ReactElement,
) {
  if (!liveTask) return inner;
  return (
    <Link href={linkToTask(sender.id)} aria-label={linkAriaLabel}>
      {inner}
    </Link>
  );
}

function SenderTaskTooltip({ sessionName, fullTitle }: SenderTaskTooltipProps) {
  if (sessionName) {
    return (
      <Trans i18nKey="task:fromSessionInTask" values={{ sessionName, fullTitle }}>
        From session <span className="font-semibold">&ldquo;{sessionName}&rdquo;</span> in task
        {" " /* Keep the task title span at Trans child index 4 for locale templates. */}
        <span className="font-semibold">&ldquo;{fullTitle}&rdquo;</span>
      </Trans>
    );
  }
  return (
    <Trans i18nKey="task:fromAgentInTask" values={{ fullTitle }}>
      From agent in task <span className="font-semibold">&ldquo;{fullTitle}&rdquo;</span>
    </Trans>
  );
}

/**
 * Renders a sender-session badge for same-task messages and a source-task
 * badge for cross-task messages. Cross-task titles update from the kanban store;
 * unloaded tasks use their captured title and render un-linked + dimmed.
 *
 * Used by chat-message rows AND by the queued-ghost row so a queued inter-task
 * prompt uses the same attribution as the final delivered message.
 */
export function SenderTaskBadge({ sender, destinationTaskId, size = "sm" }: SenderTaskBadgeProps) {
  const {
    liveTask,
    isSameTaskMessage,
    fullTitle,
    sessionName,
    sameTaskContext,
    truncated,
    sourceTaskAriaLabel,
  } = useSenderTaskBadgeModel(sender, destinationTaskId);

  const sizeClass =
    size === "xs" ? "gap-1 px-1.5 py-0.5 text-[10px]" : "gap-1.5 px-2.5 py-1 text-xs font-medium";
  const iconSize = size === "xs" ? 10 : 14;
  const badgeClassName = cn(
    "inline-flex items-center rounded-full bg-purple-500/20 text-purple-300",
    sizeClass,
    liveTask && !isSameTaskMessage && "cursor-pointer hover:bg-purple-500/30 transition-colors",
    !liveTask && "opacity-60",
  );

  if (isSameTaskMessage) {
    return (
      <Popover>
        <PopoverTrigger asChild>
          <button
            type="button"
            className={cn(
              badgeClassName,
              "cursor-pointer border-0 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
              "max-w-full min-w-0 [@media(pointer:coarse)]:min-h-11",
            )}
            data-testid="sender-task-badge"
            data-sender-task-id={sender.id}
            aria-label={sameTaskContext}
          >
            <IconRobot className="shrink-0" size={iconSize} aria-hidden="true" />
            <span className="min-w-0 truncate">{truncated}</span>
          </button>
        </PopoverTrigger>
        <PopoverContent
          data-testid="sender-task-context"
          side="top"
          align="start"
          className="break-words"
        >
          {sameTaskContext}
        </PopoverContent>
      </Popover>
    );
  }

  const inner = (
    <span
      className={badgeClassName}
      data-testid="sender-task-badge"
      data-sender-task-id={sender.id}
    >
      <IconRobot size={iconSize} /> {truncated}
    </span>
  );

  const wrapped = wrapBadgeWithTaskLink(liveTask, sender, sourceTaskAriaLabel, inner);

  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>{wrapped}</TooltipTrigger>
        <TooltipContent>
          <SenderTaskTooltip sessionName={sessionName} fullTitle={fullTitle} />
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
