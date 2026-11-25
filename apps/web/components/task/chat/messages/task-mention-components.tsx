"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { IconChecklist } from "@tabler/icons-react";
import TaskLink from "@/components/routing/task-link";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskColor } from "@/hooks/use-task-color";
import type { TaskColor } from "@/lib/task-colors";
import type { TaskMentionReference } from "@/lib/tasks/task-mention-references";
import { cn } from "@/lib/utils";

const TASK_REFERENCE_COLORS: Record<TaskColor, string> = {
  red: "border-red-500/30 bg-red-500/10 text-red-700! hover:bg-red-500/20 dark:text-red-300!",
  orange:
    "border-orange-500/30 bg-orange-500/10 text-orange-800! hover:bg-orange-500/20 dark:text-orange-300!",
  yellow:
    "border-yellow-500/30 bg-yellow-500/10 text-yellow-800! hover:bg-yellow-500/20 dark:text-yellow-300!",
  green:
    "border-green-500/30 bg-green-500/10 text-green-800! hover:bg-green-500/20 dark:text-green-300!",
  blue: "border-blue-500/30 bg-blue-500/10 text-blue-700! hover:bg-blue-500/20 dark:text-blue-300!",
  purple:
    "border-purple-500/30 bg-purple-500/10 text-purple-700! hover:bg-purple-500/20 dark:text-purple-300!",
  pink: "border-pink-500/30 bg-pink-500/10 text-pink-700! hover:bg-pink-500/20 dark:text-pink-300!",
};

type TaskMentionChipProps = Omit<
  ComponentPropsWithoutRef<typeof TaskLink>,
  "taskId" | "children"
> & {
  reference: TaskMentionReference;
  prefix?: string;
};

export const TaskMentionChip = forwardRef<HTMLAnchorElement, TaskMentionChipProps>(
  function TaskMentionChip({ reference, prefix = "@", className, ...props }, ref) {
    const { isMobile, isFinePointer } = useResponsiveBreakpoint();
    const color = useTaskColor(reference.taskId);
    return (
      <TaskLink
        {...props}
        ref={ref}
        taskId={reference.taskId}
        data-testid="task-mention-chip"
        className={cn(
          "inline-flex max-w-full cursor-pointer items-center gap-1 rounded-md border px-1.5 py-0.5 align-middle text-[0.88em] font-medium no-underline! hover:no-underline! focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
          color
            ? TASK_REFERENCE_COLORS[color]
            : "border-primary/30 bg-primary/10 text-primary! hover:bg-primary/20",
          prefix !== "" && (isMobile || !isFinePointer) ? "min-h-11" : "min-h-6",
          prefix === "" && "leading-snug",
          className,
        )}
      >
        <IconChecklist className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
        <span className="min-w-0 whitespace-normal break-words [overflow-wrap:anywhere]">
          {prefix}
          {reference.title}
        </span>
      </TaskLink>
    );
  },
);
