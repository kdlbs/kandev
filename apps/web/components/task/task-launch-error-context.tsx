"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import type { SessionRecoveryOwner } from "@/lib/session-recovery-presentation";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";

export type TaskLaunchErrorContextValue = {
  taskId: string;
  workspaceId: string;
  statusSummary?: TaskStatusSummary | null;
  repositories?: TaskRepository[];
  automaticRecovery?: SessionRecoveryOwner | null;
};

const TaskLaunchErrorContext = createContext<TaskLaunchErrorContextValue | null>(null);

export function TaskLaunchErrorProvider({
  value,
  children,
}: {
  value: TaskLaunchErrorContextValue;
  children: ReactNode;
}) {
  const statusSummary = useTaskStatusSummary(value.taskId, value.statusSummary);

  const contextValue = useMemo(() => ({ ...value, statusSummary }), [statusSummary, value]);
  return (
    <TaskLaunchErrorContext.Provider value={contextValue}>
      {children}
    </TaskLaunchErrorContext.Provider>
  );
}

export function useTaskLaunchErrorContext(): TaskLaunchErrorContextValue | null {
  return useContext(TaskLaunchErrorContext);
}
