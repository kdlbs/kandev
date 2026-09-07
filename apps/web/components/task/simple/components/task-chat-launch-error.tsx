"use client";

import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { isTaskLaunchErrorVisibleForSession } from "@/components/task/chat/types";
import { isTypedTaskLaunchError, TaskLaunchErrorEntry } from "./task-launch-error-entry";

type TaskChatLaunchErrorProps = {
  taskId: string;
  workspaceId: string;
  statusSummary?: TaskStatusSummary | null;
  /** When supplied, only render the error that belongs to this session. */
  sessionId?: string | null;
  repositories?: TaskRepository[];
};

export function TaskChatLaunchError({
  taskId,
  workspaceId,
  statusSummary,
  sessionId,
  repositories,
}: TaskChatLaunchErrorProps) {
  const candidate = statusSummary?.active_error;
  const error =
    isTypedTaskLaunchError(candidate) && isTaskLaunchErrorVisibleForSession(candidate, sessionId)
      ? candidate
      : null;

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
