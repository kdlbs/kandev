"use client";

import { useCallback, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { detachTask as requestDetachTask } from "@/lib/api";
import type { Task } from "@/lib/types/http";
import { workflowSnapshotQueryData } from "@/lib/query/workflow-snapshot-cache";
import { workspaceModeFromMetadata } from "@/lib/kanban/map-task";

type DetachTarget = {
  id: string;
  title: string;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
};

const detachRequests = new Map<string, Promise<Task>>();

function requestDetachOnce(taskId: string): Promise<Task> {
  const existing = detachRequests.get(taskId);
  if (existing) return existing;

  const request = requestDetachTask(taskId).catch((error) => {
    toast.error(error instanceof Error ? error.message : "Failed to detach task");
    throw error;
  });
  detachRequests.set(taskId, request);

  const clearRequest = () => {
    if (detachRequests.get(taskId) === request) detachRequests.delete(taskId);
  };
  void request.then(clearRequest, clearRequest);
  return request;
}

export function useDetachTask() {
  const [detachingTaskId, setDetachingTaskId] = useState<string | null>(null);

  const detachTask = useCallback((taskId: string): Promise<Task> => {
    setDetachingTaskId(taskId);
    const request = requestDetachOnce(taskId);
    const clearLocalState = () => {
      setDetachingTaskId((current) => (current === taskId ? null : current));
    };
    void request.then(clearLocalState, clearLocalState);
    return request;
  }, []);

  return { detachTask, detachingTaskId };
}

export function useTaskDetachDialog() {
  const queryClient = useQueryClient();
  const { detachTask, detachingTaskId } = useDetachTask();
  const [detachingTask, setDetachingTask] = useState<DetachTarget | null>(null);

  const handleDetachTask = useCallback(
    (taskId: string) => {
      const task = workflowSnapshotQueryData(queryClient)
        .flatMap((snapshot) => snapshot.tasks)
        .find((item) => item.id === taskId);
      if (!task?.parent_id) return;
      setDetachingTask({
        id: task.id,
        title: task.title,
        workspaceMode: workspaceModeFromMetadata(task.metadata),
      });
    },
    [queryClient],
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
