"use client";

import { useAppStore } from "@/components/state-provider";
import { useEnsureTaskSession } from "@/hooks/use-ensure-task-session";
import { useTask } from "@/hooks/use-task";
import {
  useSessionResumption,
  type TaskArchiveState,
} from "@/hooks/domains/session/use-session-resumption";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import { SessionRecoveryFeedback } from "@/components/task/ensure-session-error";
import { TaskLaunchErrorProvider } from "@/components/task/task-launch-error-context";
import { TaskSharedError } from "@/components/task/task-shared-error";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { QuickChatContent } from "./quick-chat-content";
import { useTranslation } from "react-i18next";

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
  /** Default `true`. `false` maps to `useSessionResumption`'s
   *  `skipAutomaticRecovery`, so the check-and-resume and remote-status
   *  retry effects do nothing; resume/retry/restore stay manual actions. */
  automaticRecovery?: boolean;
  /** Renders `QuickChatContent` with `minimalToolbar` (Submit and Stop, no
   *  mode or model selector) in addition to kind `config`'s own minimal
   *  toolbar. */
  hideSessionSelectors?: boolean;
  /** Any value other than `undefined`, `null` included, overrides
   *  `resolveTaskArchiveState`. */
  taskArchiveState?: TaskArchiveState;
  /** Inserted once through the composer's `chatInputRef.insertText`, not
   *  sent. See {@link useQuickChatInitialDraft}. */
  initialDraft?: string;
  /** Applied to the composer message once per submit, before
   *  `buildSubmitMessage`. See {@link useSubmitHandler}. */
  transformOutgoing?: (message: string) => string;
  /** Hides the "Environment prepared" and "Started agent" rows once the agent
   *  booted. See {@link hideSuccessfulStartupRows}. */
  hideStartupRows?: boolean;
  /** Coordinator copilot: status line while running, one chip of tool calls
   *  after. See {@link QuickChatContent}. */
  activityDisplay?: boolean;
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

/** `hideSessionSelectors` widens kind `config`'s own minimal toolbar to every
 *  kind, rather than replacing it. */
function resolveMinimalToolbar(
  hideSessionSelectors: boolean | undefined,
  kind: QuickChatSession["kind"],
): boolean {
  if (hideSessionSelectors) return true;
  return kind === "config";
}

/** Any value other than `undefined`, `null` included, overrides
 *  {@link resolveTaskArchiveState}. */
function resolveEffectiveTaskArchiveState(
  explicit: TaskArchiveState | undefined,
  taskId: string | null,
  task: { isArchived?: boolean } | null,
  quickChatTaskId: string | null,
): TaskArchiveState {
  if (explicit !== undefined) return explicit;
  return resolveTaskArchiveState(taskId, task, quickChatTaskId);
}

export function QuickChatSessionView({
  session,
  onInitialPromptAttempted,
  automaticRecovery = true,
  hideSessionSelectors,
  taskArchiveState: taskArchiveStateProp,
  initialDraft,
  transformOutgoing,
  hideStartupRows,
  activityDisplay,
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
  const taskArchiveState = resolveEffectiveTaskArchiveState(
    taskArchiveStateProp,
    taskId,
    task,
    quickChatTaskId,
  );
  const resumption = useSessionResumption(taskId, session.sessionId, taskArchiveState, {
    skipAutomaticRecovery: !automaticRecovery,
  });
  const isPassthrough = useIsQuickChatPassthrough(session.sessionId);
  const statusSummary = useTaskStatusSummary(taskId, task?.statusSummary);
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
          minimalToolbar={resolveMinimalToolbar(hideSessionSelectors, session.kind)}
          placeholderOverride={
            session.kind === "config" ? t("chat:configChatPlaceholder") : undefined
          }
          initialPrompt={session.initialPrompt}
          onInitialPromptAttempted={onInitialPromptAttempted}
          initialDraft={initialDraft}
          transformOutgoing={transformOutgoing}
          hideStartupRows={hideStartupRows}
          activityDisplay={activityDisplay}
        />
      </div>
    </div>
  );

  if (!taskId) return sessionContent;
  const launchErrorValue = {
    taskId,
    workspaceId: task?.workspaceId ?? "",
    statusSummary,
    automaticRecovery: resumption,
  };
  if (isPassthrough) {
    return (
      <TaskLaunchErrorProvider value={launchErrorValue}>
        <div className="flex min-h-0 flex-1 flex-col">
          <TaskSharedError />
          {sessionContent}
        </div>
      </TaskLaunchErrorProvider>
    );
  }
  return (
    <TaskLaunchErrorProvider value={launchErrorValue}>
      <div className="flex min-h-0 flex-1 flex-col">
        <TaskSharedError />
        {sessionContent}
      </div>
    </TaskLaunchErrorProvider>
  );
}
