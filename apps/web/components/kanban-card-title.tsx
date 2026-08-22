"use client";

import { TaskTitleHoverCard } from "@/components/task/task-title-hover-card";
import type { Task } from "@/components/kanban-card";

/** The kanban card's title, optionally wrapped in the full-title/subtasks hover card. */
export function CardTitle({ task, enableTitleHover }: { task: Task; enableTitleHover?: boolean }) {
  const title = (
    <p
      data-testid="task-card-title"
      className="text-sm font-medium leading-tight line-clamp-1 min-w-0"
    >
      {task.title}
    </p>
  );
  if (!enableTitleHover) return title;
  return (
    <TaskTitleHoverCard taskId={task.id} title={task.title}>
      {title}
    </TaskTitleHoverCard>
  );
}
