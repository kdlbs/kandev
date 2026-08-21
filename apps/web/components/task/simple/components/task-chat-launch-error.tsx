"use client";

import { useMemo } from "react";
import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import type { RunError } from "@/app/office/tasks/[id]/types";
import { hasMatchingSessionLaunchError } from "../chat-entries";
import { isTypedTaskLaunchError, TaskLaunchErrorEntry } from "./task-launch-error-entry";

type TaskChatLaunchErrorProps = {
  taskId: string;
  workspaceId: string;
  statusSummary?: TaskStatusSummary | null;
  runErrors: RunError[];
  repositories?: TaskRepository[];
};

export function TaskChatLaunchError({
  taskId,
  workspaceId,
  statusSummary,
  runErrors,
  repositories,
}: TaskChatLaunchErrorProps) {
  const error = useMemo(() => {
    const candidate = statusSummary?.active_error;
    if (!isTypedTaskLaunchError(candidate)) return null;
    if (hasMatchingSessionLaunchError(candidate.session_id, candidate.stamp, runErrors)) {
      return null;
    }
    return candidate;
  }, [runErrors, statusSummary]);

  if (!error) return null;
  return (
    <TaskLaunchErrorEntry
      taskId={taskId}
      workspaceId={workspaceId}
      error={error}
      repositories={repositories}
    />
  );
}
