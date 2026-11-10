"use client";

import { IconChecklist } from "@tabler/icons-react";
import TaskLink from "@/components/routing/task-link";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { TaskMentionReference } from "@/lib/tasks/task-mention-references";
import { cn } from "@/lib/utils";

export function TaskMentionChip({ reference }: { reference: TaskMentionReference }) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  return (
    <TaskLink
      taskId={reference.taskId}
      data-testid="task-mention-chip"
      className={cn(
        "inline-flex max-w-full cursor-pointer items-center gap-1 rounded-md border border-sky-500/30 bg-sky-500/15 px-1.5 py-0.5 align-middle text-[0.88em] font-medium text-sky-950! no-underline! hover:no-underline! hover:bg-sky-500/25 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-500 dark:text-sky-100!",
        isMobile || !isFinePointer ? "min-h-11" : "min-h-6",
      )}
    >
      <IconChecklist className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span className="min-w-0 whitespace-normal break-words [overflow-wrap:anywhere]">
        @{reference.title}
      </span>
    </TaskLink>
  );
}
