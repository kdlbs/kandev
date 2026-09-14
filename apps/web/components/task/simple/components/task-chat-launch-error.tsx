"use client";

import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { isTaskLaunchErrorVisibleForSession } from "@/components/task/chat/types";
import { selectSessionRecoveryError } from "@/lib/session-recovery-presentation";
import { SessionBootstrapRecoveryCard } from "@/components/task/chat/session-bootstrap-recovery-card";
import { isTypedTaskLaunchError, TaskLaunchErrorEntry } from "./task-launch-error-entry";
import { useTaskLaunchErrorContext } from "@/components/task/task-launch-error-context";

type TaskChatLaunchErrorProps = {
  taskId: string;
  workspaceId: string;
  statusSummary?: TaskStatusSummary | null;
  /** When supplied, only render the error that belongs to this session. */
  sessionId?: string | null;
  sessionMetadata?: Record<string, unknown> | null;
  repositories?: TaskRepository[];
};

export function TaskChatLaunchError({
  taskId,
  workspaceId,
  statusSummary,
  sessionId,
  sessionMetadata,
  repositories,
}: TaskChatLaunchErrorProps) {
  const launchErrorContext = useTaskLaunchErrorContext();
  const candidate = statusSummary?.active_error;
  const bootstrapError = selectSessionRecoveryError(candidate, sessionId, sessionMetadata);
  if (bootstrapError && sessionId) {
    return (
      <SessionBootstrapRecoveryCard
        taskId={taskId}
        sessionId={sessionId}
        workspaceId={workspaceId}
        error={bootstrapError}
        automaticRecovery={launchErrorContext?.automaticRecovery}
      />
    );
  }
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
