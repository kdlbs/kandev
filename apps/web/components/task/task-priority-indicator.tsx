"use client";

import { IconAlertTriangle, IconArrowDown, IconArrowUp } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { isTaskPriority, TASK_PRIORITY_LABEL_KEYS } from "@/lib/tasks/task-priority";
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

/** Show only non-default priorities so ordinary task rows stay visually quiet. */
export function TaskPriorityIndicator({
  priority,
  testId = "task-priority-indicator",
}: {
  priority?: TaskPriority | string | null;
  testId?: string;
}) {
  const { t } = useTranslation();
  if (!isTaskPriority(priority) || !isIndicatedPriority(priority)) return null;

  const Icon = PRIORITY_ICONS[priority];
  const label = t(TASK_PRIORITY_LABEL_KEYS[priority]);

  return (
    <span
      data-testid={testId}
      role="img"
      aria-label={t("kanban:priorityIndicatorLabel", { priority: label })}
      title={label}
      className={cn("inline-flex shrink-0 items-center", PRIORITY_COLORS[priority])}
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
    </span>
  );
}
