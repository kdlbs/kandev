"use client";

import { useAppStore } from "@/components/state-provider";
import { useEnsureTaskSession } from "@/hooks/use-ensure-task-session";
import { useTask } from "@/hooks/use-task";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import { SessionRecoveryFeedback } from "@/components/task/ensure-session-error";
import { SessionBootstrapRecoveryCard } from "@/components/task/chat/session-bootstrap-recovery-card";
import { selectSessionRecoveryError } from "@/lib/session-recovery-presentation";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { QuickChatContent } from "./quick-chat-content";
import { useTranslation } from "react-i18next";

// i18n-exempt: internal recovery identity token, not user-facing copy.
const GENERIC_RECOVERY_OUTCOME = "feedback";

function useIsQuickChatPassthrough(sessionId: string) {
  return useAppStore((state) => {
    const session = state.taskSessions.items[sessionId];
    if (typeof session?.is_passthrough === "boolean") return session.is_passthrough;
    const profileId =
      session?.agent_profile_id ??
      state.quickChat.sessions.find((item) => item.sessionId === sessionId)?.agentProfileId;
    if (!profileId) return false;
    return state.agentProfiles.items.find((profile) => profile.id === profileId)?.cli_passthrough;
  });
}

type QuickChatSessionViewProps = {
  session: QuickChatSession;
  onInitialPromptAttempted?: () => void;
};

function resolveTaskArchiveState(
  taskId: string | null,
  task: { isArchived?: boolean } | null,
  quickChatTaskId: string | null,
): boolean | null {
  if (!taskId) return null;
  if (task) return task.isArchived === true;
  // Ephemeral Quick Chat tasks are intentionally absent from the kanban task
  // cache. Their hydrated tab is the authoritative live-task source.
  return quickChatTaskId === taskId ? false : null;
}

function resolveRecoveryRevealKey(
  sessionId: string,
  bootstrapRecoveryError: { stamp: string; occurred_at: string } | null,
  hasRecoveryFeedback: boolean,
  recoveryAttemptId: number | undefined,
  recoveryOutcome: string | null,
): string | null {
  if (bootstrapRecoveryError) {
    return `${sessionId}:${bootstrapRecoveryError.stamp || bootstrapRecoveryError.occurred_at}`;
  }
  if (!hasRecoveryFeedback) return null;
  return `${sessionId}:recovery:${recoveryAttemptId ?? 0}:${recoveryOutcome ?? GENERIC_RECOVERY_OUTCOME}`;
}

function hasRecoveryFeedback(state: {
  error: string | null;
  notice: string | null;
  recoveryFailure: unknown;
}): boolean {
  return Boolean(state.error) || Boolean(state.notice) || state.recoveryFailure !== null;
}

function recoveryOutcome(failure: { outcome: string } | null): string | null {
  if (!failure) return null;
  return failure.outcome;
}

export function QuickChatSessionView({
  session,
  onInitialPromptAttempted,
}: QuickChatSessionViewProps) {
  const { t } = useTranslation();
  // A tab can arrive from a task event, which carries no session payload.
  // Fetch the row on open so such a tab is usable, not just visible.
  useEnsureTaskSession(session.sessionId);
  const taskSession = useAppStore((state) => state.taskSessions.items[session.sessionId] ?? null);
  const quickChatTaskId = useAppStore(
    (state) =>
      state.quickChat.sessions.find((item) => item.sessionId === session.sessionId)?.taskId ?? null,
  );
  const taskId = taskSession ? (session.taskId ?? taskSession.task_id ?? null) : quickChatTaskId;
  const task = useTask(taskId);
  const taskArchiveState = resolveTaskArchiveState(taskId, task, quickChatTaskId);
  const resumption = useSessionResumption(taskId, session.sessionId, taskArchiveState);
  const isPassthrough = useIsQuickChatPassthrough(session.sessionId);
  const statusSummary = useTaskStatusSummary(taskId, task?.statusSummary);
  const bootstrapRecoveryError = taskId
    ? selectSessionRecoveryError(
        statusSummary?.active_error,
        session.sessionId,
        taskSession?.metadata,
      )
    : null;
  const recoveryFeedback = (
    <SessionRecoveryFeedback
      error={resumption.error}
      notice={resumption.notice}
      recoveryFailure={resumption.recoveryFailure}
      onRetry={() => void resumption.resumeSession()}
      retryDisabled={
        resumption.resumptionState === "checking" || resumption.resumptionState === "resuming"
      }
    />
  );
  const recoverySurface = bootstrapRecoveryError ? (
    <SessionBootstrapRecoveryCard
      taskId={taskId!}
      sessionId={session.sessionId}
      workspaceId={task?.workspaceId ?? null}
      error={bootstrapRecoveryError}
      automaticRecovery={resumption}
    />
  ) : (
    recoveryFeedback
  );
  const recoveryRevealKey = resolveRecoveryRevealKey(
    session.sessionId,
    bootstrapRecoveryError,
    hasRecoveryFeedback(resumption),
    resumption.recoveryAttemptId,
    recoveryOutcome(resumption.recoveryFailure),
  );
  if (isPassthrough) {
    return (
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {recoverySurface}
        <div className="min-h-0 flex-1">
          <PassthroughTerminal key={session.sessionId} sessionId={session.sessionId} mode="agent" />
        </div>
      </div>
    );
  }
  const isConfig = session.kind === "config";
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1 flex-col">
        <QuickChatContent
          sessionId={session.sessionId}
          minimalToolbar={isConfig}
          placeholderOverride={isConfig ? t("chat:configChatPlaceholder") : undefined}
          initialPrompt={session.initialPrompt}
          onInitialPromptAttempted={onInitialPromptAttempted}
          recoveryContent={recoverySurface}
          recoveryRevealKey={recoveryRevealKey}
        />
      </div>
    </div>
  );
}
