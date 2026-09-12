"use client";

import { TaskTitleHoverCard } from "@/components/task/task-title-hover-card";
import { useIsTitleTruncated } from "@/hooks/use-is-title-truncated";
import type { Task } from "@/components/kanban-card";

/** The kanban card's title, optionally wrapped in the full-title/subtasks hover card. */
export function CardTitle({ task, enableTitleHover }: { task: Task; enableTitleHover?: boolean }) {
  const { ref, isTruncated } = useIsTitleTruncated<HTMLParagraphElement>(task.title);
  const title = (
    <p
      ref={ref}
      data-testid="task-card-title"
      className="text-sm font-medium leading-tight line-clamp-1 min-w-0"
    >
      {task.title}
    </p>
  );
  if (!enableTitleHover) return title;
  return (
    <TaskTitleHoverCard
      taskId={task.id}
      title={task.title}
      description={task.description}
      parentTaskId={task.parentTaskId}
      isTitleTruncated={isTruncated}
    >
      {title}
    </TaskTitleHoverCard>
  );
}
