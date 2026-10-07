import {
  getSessionRecoveryRetry,
  SessionRecoveryFeedback,
} from "@/components/task/ensure-session-error";
import { SessionBootstrapRecoveryCard } from "@/components/task/chat/session-bootstrap-recovery-card";
import type { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import type { TaskStatusSummaryActiveError } from "@/lib/types/task-status-summary";

export function TaskPageRecoveryFeedback({
  ownedByChat,
  taskId,
  sessionId,
  resumption,
  bootstrapRecoveryError,
  workspaceId,
  isPassthrough,
}: {
  taskId: string;
  sessionId: string | null;
  resumption: ReturnType<typeof useSessionResumption>;
  ownedByChat: boolean;
  bootstrapRecoveryError: TaskStatusSummaryActiveError | null;
  workspaceId: string | null;
  isPassthrough: boolean;
}) {
  if (bootstrapRecoveryError && sessionId && isPassthrough) {
    return (
      <SessionBootstrapRecoveryCard
        taskId={taskId}
        sessionId={sessionId}
        workspaceId={workspaceId}
        error={bootstrapRecoveryError}
        automaticRecovery={resumption}
      />
    );
  }
  if (bootstrapRecoveryError) {
    return null;
  }
  return (
    <SessionRecoveryFeedback
      ownedByChat={ownedByChat}
      error={resumption.error}
      notice={resumption.notice}
      recoveryFailure={resumption.recoveryFailure}
      onRetry={getSessionRecoveryRetry(resumption)}
      retryDisabled={
        resumption.resumptionState === "checking" || resumption.resumptionState === "resuming"
      }
      workspaceId={workspaceId}
    />
  );
}
