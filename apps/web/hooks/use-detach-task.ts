"use client";

import { useCallback, useRef, useState } from "react";
import type { StoreApi } from "zustand";
import { toast } from "sonner";
import { detachTask as requestDetachTask } from "@/lib/api";
import type { Task } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";

type DetachTarget = {
  id: string;
  title: string;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
};

export function useDetachTask() {
  const inFlight = useRef(new Map<string, Promise<Task>>());
  const [detachingTaskId, setDetachingTaskId] = useState<string | null>(null);

  const detachTask = useCallback((taskId: string): Promise<Task> => {
    const existing = inFlight.current.get(taskId);
    if (existing) return existing;

    setDetachingTaskId(taskId);
    const request = requestDetachTask(taskId)
      .catch((error) => {
        toast.error(error instanceof Error ? error.message : "Failed to detach task");
        throw error;
      })
      .finally(() => {
        inFlight.current.delete(taskId);
        setDetachingTaskId((current) => (current === taskId ? null : current));
      });
    inFlight.current.set(taskId, request);
    return request;
  }, []);

  return { detachTask, detachingTaskId };
}

export function useTaskDetachDialog(store: StoreApi<AppState>) {
  const { detachTask, detachingTaskId } = useDetachTask();
  const [detachingTask, setDetachingTask] = useState<DetachTarget | null>(null);

  const handleDetachTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      if (!task?.parentTaskId) return;
      setDetachingTask({
        id: task.id,
        title: task.title,
        workspaceMode: task.workspaceMode,
      });
    },
    [store],
  );

  const handleDetachConfirm = useCallback(async () => {
    if (!detachingTask) return;
    try {
      await detachTask(detachingTask.id);
      setDetachingTask(null);
    } catch (error) {
      console.error("Failed to detach task:", error);
    }
  }, [detachTask, detachingTask]);

  return {
    detachingTask,
    setDetachingTask,
    detachingTaskId,
    handleDetachTask,
    handleDetachConfirm,
  };
}
