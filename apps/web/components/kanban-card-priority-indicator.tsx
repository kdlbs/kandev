"use client";

import { IconAlertTriangle, IconArrowDown, IconArrowUp } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { isKanbanPriority, KANBAN_PRIORITY_LABEL_KEYS } from "@/lib/kanban/task-priority";
import type { TaskPriority } from "@/lib/types/http";
import { cn } from "@/lib/utils";

type IndicatedPriority = Exclude<TaskPriority, "medium">;

const PRIORITY_ICONS: Record<IndicatedPriority, typeof IconAlertTriangle> = {
  critical: IconAlertTriangle,
  high: IconArrowUp,
  low: IconArrowDown,
};

const PRIORITY_COLORS: Record<IndicatedPriority, string> = {
  critical: "text-red-600 dark:text-red-400",
  high: "text-amber-600 dark:text-amber-500",
  low: "text-sky-600 dark:text-sky-400",
};

function isIndicatedPriority(priority: TaskPriority): priority is IndicatedPriority {
  return priority !== "medium";
}

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
  const { t } = useTranslation();
  if (!isKanbanPriority(priority) || !isIndicatedPriority(priority)) return null;

  const Icon = PRIORITY_ICONS[priority];
  const label = t(KANBAN_PRIORITY_LABEL_KEYS[priority]);

  return (
    <span
      data-testid="kanban-card-priority-indicator"
      role="img"
      aria-label={t("kanban:priorityIndicatorLabel", { priority: label })}
      title={label}
      className={cn("shrink-0", PRIORITY_COLORS[priority])}
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
    </span>
  );
}
