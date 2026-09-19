"use client";

import { createContext, useCallback, useContext, useMemo, useRef, type ReactNode } from "react";
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
  /** Claims one assertive announcement for a task error stamp. */
  claimTaskErrorAnnouncement?: (stamp: string) => boolean;
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
  const announcedTaskErrorRef = useRef<string | null>(null);
  const claimTaskErrorAnnouncement = useCallback(
    (stamp: string) => {
      const key = `${value.taskId}:${stamp}`;
      if (!stamp || announcedTaskErrorRef.current === key) return false;
      announcedTaskErrorRef.current = key;
      return true;
    },
    [value.taskId],
  );

  const contextValue = useMemo(
    () => ({ ...value, statusSummary, claimTaskErrorAnnouncement }),
    [claimTaskErrorAnnouncement, statusSummary, value],
  );
  return (
    <TaskLaunchErrorContext.Provider value={contextValue}>
      {children}
    </TaskLaunchErrorContext.Provider>
  );
}

export function useTaskLaunchErrorContext(): TaskLaunchErrorContextValue | null {
  return useContext(TaskLaunchErrorContext);
}
