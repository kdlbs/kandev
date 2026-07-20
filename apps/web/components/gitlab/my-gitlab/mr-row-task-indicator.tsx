"use client";

import type { TaskMR } from "@/lib/types/gitlab";
import { TaskRowIndicator } from "@/components/github/my-github/task-row-indicator";

export function MRRowTaskIndicator({ tasks }: { tasks: TaskMR[] | undefined }) {
  return (
    <TaskRowIndicator
      tasks={tasks?.map((association) => ({
        id: association.id,
        taskId: association.task_id,
        fallbackTitle: association.mr_title,
      }))}
      testIdPrefix="gitlab-mr-row-task-indicator"
      emptyLabel="No task created yet"
    />
  );
}
