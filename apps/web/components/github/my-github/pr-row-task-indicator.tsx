"use client";

import type { TaskPR } from "@/lib/types/github";
import { TaskRowIndicator } from "./task-row-indicator";
import { useTranslation } from "react-i18next";

type PRRowTaskIndicatorProps = {
  tasks: TaskPR[] | undefined;
};

export function PRRowTaskIndicator({ tasks }: PRRowTaskIndicatorProps) {
  const { t } = useTranslation();
  return (
    <TaskRowIndicator
      tasks={tasks?.map((task) => ({
        id: task.id,
        taskId: task.task_id,
        fallbackTitle: task.pr_title,
      }))}
      testIdPrefix="pr-row-task-indicator"
      emptyLabel={t("github:noTaskCreatedYet")}
    />
  );
}
