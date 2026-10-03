"use client";

import { useAppStore } from "@/components/state-provider";
import { useEnsureTaskSession } from "@/hooks/use-ensure-task-session";
import { useTask } from "@/hooks/use-task";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import { SessionRecoveryFeedback } from "@/components/task/ensure-session-error";
import { TaskLaunchErrorProvider } from "@/components/task/task-launch-error-context";
import { TaskSharedError } from "@/components/task/task-shared-error";
import type { TaskSession } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { QuickChatContent } from "./quick-chat-content";
import { readLastAgentError } from "@/lib/session-last-agent-error";
import { useTranslation } from "react-i18next";
import { ConfigChatRestartStatus } from "@/components/config-chat/config-chat-restart-status";

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

function preventQuickChatAutoResume(
  sessionId: string,
  taskId: string | null,
  taskSession: TaskSession | null,
  summary: TaskStatusSummary | null | undefined,
): boolean {
  return Boolean(
    (taskId && !taskSession) ||
    (summary?.active_error &&
      (summary.active_error.scope === "task" || summary.active_error.session_id === sessionId)) ||
    summary?.task_error ||
    readLastAgentError(taskSession?.metadata),
  );
}

export function QuickChatSessionView({
  session,
  onInitialPromptAttempted,
}: QuickChatSessionViewProps) {
  const restart = useAppStore((state) => state.quickChat.configChatRestarts?.[session.workspaceId]);
  if (session.kind === "config" && restart) {
    return <ConfigChatRestartStatus workspaceId={session.workspaceId} />;
  }
  return (
    <ActiveQuickChatSessionView
      session={session}
      onInitialPromptAttempted={onInitialPromptAttempted}
    />
  );
}

function ActiveQuickChatSessionView({
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
  const statusSummary = useTaskStatusSummary(taskId, task?.statusSummary);
  const taskArchiveState = resolveTaskArchiveState(taskId, task, quickChatTaskId);
  const resumption = useSessionResumption(taskId, session.sessionId, taskArchiveState, {
    preventAutoResume: preventQuickChatAutoResume(
      session.sessionId,
      taskId,
      taskSession,
      statusSummary,
    ),
  });
  const isPassthrough = useIsQuickChatPassthrough(session.sessionId);
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
  const sessionContent = isPassthrough ? (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      {recoveryFeedback}
      <div className="min-h-0 flex-1">
        <PassthroughTerminal key={session.sessionId} sessionId={session.sessionId} mode="agent" />
      </div>
    </div>
  ) : (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1 flex-col">
        <QuickChatContent
          sessionId={session.sessionId}
          minimalToolbar={session.kind === "config"}
          placeholderOverride={
            session.kind === "config" ? t("chat:configChatPlaceholder") : undefined
          }
          initialPrompt={session.initialPrompt}
          onInitialPromptAttempted={onInitialPromptAttempted}
        />
      </div>
    </div>
  );

  if (!taskId) return sessionContent;

  return (
    <TaskLaunchErrorProvider
      value={{
        taskId,
        workspaceId: task?.workspaceId ?? "",
        statusSummary,
        automaticRecovery: resumption,
      }}
    >
      <div className="flex min-h-0 flex-1 flex-col">
        <TaskSharedError />
        {sessionContent}
      </div>
    </TaskLaunchErrorProvider>
  );
}
