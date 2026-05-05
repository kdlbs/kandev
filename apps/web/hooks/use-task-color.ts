"use client";

import { useCallback, useSyncExternalStore } from "react";
import {
  getTaskColor,
  setTaskColor,
  TASK_COLORS_CHANGED_EVENT,
  type TaskColor,
} from "@/lib/task-colors";

function subscribe(cb: () => void): () => void {
  if (typeof window === "undefined") return () => {};
  window.addEventListener(TASK_COLORS_CHANGED_EVENT, cb);
  window.addEventListener("storage", cb);
  return () => {
    window.removeEventListener(TASK_COLORS_CHANGED_EVENT, cb);
    window.removeEventListener("storage", cb);
  };
}

const getServerSnapshot = (): TaskColor | null => null;

export function useTaskColor(taskId: string | undefined): TaskColor | null {
  const getSnapshot = useCallback(() => (taskId ? getTaskColor(taskId) : null), [taskId]);
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

export function useSetTaskColor(): (taskId: string, color: TaskColor | null) => void {
  return useCallback((taskId, color) => setTaskColor(taskId, color), []);
}
