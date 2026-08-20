"use client";

import { createContext, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { getSessionStorage, setSessionStorage } from "@/lib/local-storage";
import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";
import { isTypedTaskLaunchError } from "./simple/components/task-launch-error-entry";

// i18n-exempt: persisted browser storage key, not user-facing copy.
const LAUNCH_ERROR_TOASTS_KEY = "kandev.task-launch-error-toasts";

export type TaskLaunchErrorContextValue = {
  taskId: string;
  workspaceId: string;
  statusSummary?: TaskStatusSummary | null;
  repositories?: TaskRepository[];
};

const TaskLaunchErrorContext = createContext<TaskLaunchErrorContextValue | null>(null);

export function TaskLaunchErrorProvider({
  value,
  children,
}: {
  value: TaskLaunchErrorContextValue;
  children: ReactNode;
}) {
  const { toast } = useToast();
  const shownToastStampsRef = useRef(new Set<string>());
  const statusSummary = useTaskStatusSummary(value.taskId, value.statusSummary);

  useEffect(() => {
    const activeError = statusSummary?.active_error;
    if (!isTypedTaskLaunchError(activeError)) return;

    const localStamp = `${value.taskId}:${activeError.stamp}`;
    if (shownToastStampsRef.current.has(localStamp)) return;
    const shownByTask = getSessionStorage<Record<string, string[]>>(LAUNCH_ERROR_TOASTS_KEY, {});
    if ((shownByTask[value.taskId] ?? []).includes(activeError.stamp)) {
      shownToastStampsRef.current.add(localStamp);
      return;
    }

    shownToastStampsRef.current.add(localStamp);
    setSessionStorage(LAUNCH_ERROR_TOASTS_KEY, {
      ...shownByTask,
      [value.taskId]: [...(shownByTask[value.taskId] ?? []), activeError.stamp].slice(-20),
    });
    toast({
      title: t("task:taskFailedToStart"),
      description: t("task:launchFailedSeeDetails"),
      variant: "error",
    });
  }, [
    statusSummary?.active_error?.category,
    statusSummary?.active_error?.stamp,
    toast,
    value.taskId,
  ]);

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
