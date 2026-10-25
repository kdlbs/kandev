"use client";

import { TaskPriorityIndicator } from "@/components/task/task-priority-indicator";
import type { TaskPriority } from "@/lib/types/http";

/**
 * `medium` is the default and majority case, so it renders no indicator —
 * the signal is the exception, not decoration on every card. An absent or
 * unrecognized value renders nothing rather than the raw token.
 */
export function KanbanCardPriorityIndicator({
  priority,
}: {
  priority?: TaskPriority | string | null;
}) {
  return <TaskPriorityIndicator priority={priority} testId="kanban-card-priority-indicator" />;
}
