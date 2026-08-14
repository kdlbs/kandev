"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskSubtasks } from "@/hooks/domains/kanban/use-task-subtasks";
import { TaskSubtaskRow } from "./task-subtask-row";

const MAX_VISIBLE_SUBTASKS = 12;

function SubtasksSection({ taskId }: { taskId: string }) {
  const { t } = useTranslation();
  const subtasks = useTaskSubtasks(taskId);
  if (subtasks.length === 0) return null;

  const visible = subtasks.slice(0, MAX_VISIBLE_SUBTASKS);
  const overflowCount = subtasks.length - visible.length;

  return (
    <div
      className="mt-2.5 border-t border-border/60 pt-2.5"
      data-testid="task-title-hover-subtasks"
    >
      <div className="text-[11px] font-medium text-muted-foreground">
        {t("task:subtasksHeading", { count: subtasks.length })}
      </div>
      <div className="mt-1.5 space-y-0.5">
        {visible.map((subtask) => (
          <TaskSubtaskRow key={subtask.id} subtask={subtask} />
        ))}
        {overflowCount > 0 && (
          <div className="px-1 py-1 text-xs text-muted-foreground">
            {t("task:moreNotShown", { count: overflowCount })}
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * Wraps a task title with a hover card showing the full, untruncated title
 * in bold plus a clickable list of direct subtasks. Uses HoverCard (not
 * Tooltip) because the UI package's TooltipContent is pointer-events-none
 * with disableHoverableContent, which makes clickable rows impossible there.
 * Bails out to plain children on a coarse pointer — no touch surface is added
 * for this hover (mobile-parity out-of-scope per plan).
 */
export function TaskTitleHoverCard({
  taskId,
  title,
  children,
  side = "bottom",
  align = "start",
}: {
  taskId: string;
  title: string;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
}) {
  const { isFinePointer } = useResponsiveBreakpoint();

  if (!isFinePointer) return <>{children}</>;

  return (
    <HoverCard openDelay={200} closeDelay={100}>
      <HoverCardTrigger asChild>{children}</HoverCardTrigger>
      <HoverCardContent
        side={side}
        align={align}
        data-testid="task-title-hover-card"
        className="w-80 max-w-[calc(100vw-1rem)] max-h-80 overflow-y-auto p-3"
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <div className="text-pretty break-words text-sm font-semibold leading-snug text-foreground [overflow-wrap:anywhere]">
          {title}
        </div>
        <SubtasksSection taskId={taskId} />
      </HoverCardContent>
    </HoverCard>
  );
}
