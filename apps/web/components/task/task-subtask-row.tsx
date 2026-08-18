"use client";

import { IconGitMerge, IconGitPullRequest } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { aggregatePRStatusColor } from "@/components/github/pr-task-icon";
import { aggregateMRStatusColor } from "@/components/gitlab/mr-task-icon";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { linkToTask } from "@/lib/links";
import { getTaskStateIcon } from "@/lib/ui/state-icons";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import { cn } from "@/lib/utils";
import type { TaskSubtask } from "@/hooks/domains/kanban/use-task-subtasks";

/**
 * A presentational CI glyph — not PRTaskIcon/MRTaskIcon. Those are Tooltip
 * roots, and nesting one inside hover-card content is the pattern
 * apps/web/AGENTS.md warns against (a Tooltip can close and unmount its
 * parent HoverCard before the pointer reaches it).
 */
function SubtaskCIGlyph({ taskId }: { taskId: string }) {
  const prs = useAppStore((state) => state.taskPRs.byTaskId[taskId]) ?? [];
  const mrs = useTaskMRs(taskId);
  if (prs.length > 0) {
    return (
      <IconGitPullRequest
        aria-hidden="true"
        className={cn("size-3.5 shrink-0", aggregatePRStatusColor(prs))}
      />
    );
  }
  if (mrs.length > 0) {
    return (
      <IconGitMerge
        aria-hidden="true"
        className={cn("size-3.5 shrink-0", aggregateMRStatusColor(mrs))}
      />
    );
  }
  return null;
}

export function TaskSubtaskRow({ subtask }: { subtask: TaskSubtask }) {
  return (
    <Link
      href={linkToTask(subtask.id)}
      data-testid={`task-subtask-row-${subtask.id}`}
      className="flex cursor-pointer items-center gap-1.5 rounded-sm px-1 py-1 hover:bg-accent"
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") event.stopPropagation();
      }}
    >
      {/* The state label names the icon, not the link: an aria-label on the
          anchor would replace its accessible name, so every row would announce
          as its lifecycle state with the subtask title never read out. */}
      <span role="img" aria-label={formatTaskStateLabel(subtask.state)} className="inline-flex">
        {getTaskStateIcon(subtask.state, "h-3.5 w-3.5 shrink-0")}
      </span>
      <SubtaskCIGlyph taskId={subtask.id} />
      <span className="min-w-0 flex-1 text-pretty break-words text-xs leading-snug [overflow-wrap:anywhere]">
        {subtask.title}
      </span>
    </Link>
  );
}
