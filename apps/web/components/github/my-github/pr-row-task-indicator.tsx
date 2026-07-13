"use client";

import type { TaskPR } from "@/lib/types/github";
import { TaskRowIndicator } from "./task-row-indicator";

type PRRowTaskIndicatorProps = {
  tasks: TaskPR[] | undefined;
};

export function PRRowTaskIndicator({ tasks }: PRRowTaskIndicatorProps) {
  return (
    <TaskRowIndicator
      tasks={tasks?.map((task) => ({
        id: task.task_id,
        taskId: task.task_id,
        fallbackTitle: task.pr_title,
      }))}
      testIdPrefix="pr-row-task-indicator"
      emptyLabel="No task created yet"
    />
  );
}
