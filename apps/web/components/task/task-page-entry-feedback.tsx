"use client";

import { EnsureSessionErrorBanner } from "@/components/task/ensure-session-error";
import { TaskMoveErrorBanner } from "@/components/task/task-move-error-banner";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";

export function TaskPageEntryFeedback({
  taskMoveError,
  ensureSession,
  workspaceId,
}: {
  taskMoveError: unknown;
  ensureSession: UseEnsureTaskSessionResult;
  workspaceId: string | null;
}) {
  return (
    <>
      {taskMoveError !== null && <TaskMoveErrorBanner error={taskMoveError} />}
      {ensureSession.status === "error" && (
        <EnsureSessionErrorBanner
          error={ensureSession.error}
          onRetry={ensureSession.retry}
          workspaceId={workspaceId}
        />
      )}
    </>
  );
}
