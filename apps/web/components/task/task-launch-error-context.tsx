"use client";

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { fetchTask } from "@/lib/api";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { getSessionStorage, setSessionStorage } from "@/lib/local-storage";
import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

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
  const [fetchedSummary, setFetchedSummary] = useState<TaskStatusSummary | null | undefined>(
    value.statusSummary,
  );

  useEffect(() => {
    const activeError = fetchedSummary?.active_error;
    if (!activeError?.stamp) return;

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
  }, [fetchedSummary?.active_error?.stamp, toast, value.taskId]);

  useEffect(() => {
    let cancelled = false;
    const refresh = async () => {
      try {
        const task = await fetchTask(value.taskId, { cache: "no-store" });
        if (cancelled) return;
        if (task.status_summary) setFetchedSummary(task.status_summary);
        if (task.status_summary?.active_error && pollID !== undefined) {
          window.clearInterval(pollID);
        }
      } catch {
        // Hydrated task data remains the fallback when the refresh is unavailable.
      }
    };

    setFetchedSummary(value.statusSummary);
    const pollID = window.setInterval(() => void refresh(), 1_000);
    void refresh();
    const stopID = window.setTimeout(() => window.clearInterval(pollID), 30_000);
    return () => {
      cancelled = true;
      window.clearInterval(pollID);
      window.clearTimeout(stopID);
    };
  }, [value.statusSummary, value.taskId]);

  const contextValue = useMemo(
    () => ({ ...value, statusSummary: fetchedSummary }),
    [fetchedSummary, value],
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
