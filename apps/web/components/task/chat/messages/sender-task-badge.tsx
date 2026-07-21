"use client";

import Link from "@/components/routing/app-link";
import { IconRobot } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { linkToTask } from "@/lib/links";

export type SenderTaskInfo = {
  id: string;
  /** Title captured when the message was queued/sent. Survives the sender
   * task being renamed, archived, or unloaded from the live kanban state. */
  snapshotTitle: string;
  /** Sender session id, when the message came from a specific session. */
  sessionId?: string;
  /** Sender session's user-supplied name captured at send time ("" when unnamed). */
  sessionName?: string;
};

const SENDER_TITLE_MAX = 24;

function truncateTitle(title: string): string {
  if (title.length <= SENDER_TITLE_MAX) return title;
  return title.slice(0, SENDER_TITLE_MAX - 1).trimEnd() + "…";
}

type SenderTaskBadgeProps = {
  sender: SenderTaskInfo;
  /** Optional override for the badge size — defaults to "sm" (chat bubbles). */
  size?: "xs" | "sm";
};

/**
 * Renders a purple "From {task}" badge for inter-task agent messages. The badge
 * live-resolves the sender task title from the kanban store so renames flow
 * through; when the source task isn't loaded (cross-workspace, archived) it
 * falls back to the snapshot title and renders un-linked + dimmed.
 *
 * Used by chat-message rows AND by the queued-ghost row so a queued inter-task
 * prompt shows the same provenance affordance as the final delivered message.
 */
export function SenderTaskBadge({ sender, size = "sm" }: SenderTaskBadgeProps) {
  const liveTask = useTaskById(sender.id);
  // Live-resolve the sender session's name when it's in the store (sibling
  // sessions on the loaded task), falling back to the send-time snapshot.
  const liveSessionName = useAppStore((state) =>
    sender.sessionId ? (state.taskSessions.items[sender.sessionId]?.name ?? null) : null,
  );
  const fullTitle = liveTask?.title || sender.snapshotTitle || "(unknown task)";
  const sessionName = liveSessionName ?? sender.sessionName ?? "";
  const truncated = sessionName
    ? `${truncateTitle(fullTitle)} · ${truncateTitle(sessionName)}`
    : truncateTitle(fullTitle);

  const sizeClass =
    size === "xs" ? "gap-1 px-1.5 py-0.5 text-[10px]" : "gap-1.5 px-2.5 py-1 text-xs font-medium";
  const iconSize = size === "xs" ? 10 : 14;

  const inner = (
    <span
      className={cn(
        "inline-flex items-center rounded-full bg-purple-500/20 text-purple-300",
        sizeClass,
        liveTask && "cursor-pointer hover:bg-purple-500/30 transition-colors",
        !liveTask && "opacity-60",
      )}
      data-testid="sender-task-badge"
      data-sender-task-id={sender.id}
    >
      <IconRobot size={iconSize} /> {truncated}
    </span>
  );

  const wrapped = liveTask ? (
    <Link href={linkToTask(sender.id)} aria-label={`Open source task ${fullTitle}`}>
      {inner}
    </Link>
  ) : (
    inner
  );

  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>{wrapped}</TooltipTrigger>
        <TooltipContent>
          {sessionName ? (
            <>
              From session <span className="font-semibold">&ldquo;{sessionName}&rdquo;</span> in
              task <span className="font-semibold">&ldquo;{fullTitle}&rdquo;</span>
            </>
          ) : (
            <>
              From agent in task <span className="font-semibold">&ldquo;{fullTitle}&rdquo;</span>
            </>
          )}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
