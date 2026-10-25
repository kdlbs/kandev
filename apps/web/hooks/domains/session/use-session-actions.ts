"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { deleteTaskAfterUserAction } from "@/lib/api/domains/kanban-api";
import { resolveSessionDeletionTarget } from "@/lib/session/session-deletion";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useSessionRestart } from "./use-session-restart";

export { isSessionStoppable, isSessionDeletable, isSessionResumable } from "./session-action-state";

type SessionActionsArgs = {
  sessionId: string | null | undefined;
  taskId: string | null;
  /** Optional callback after a successful delete (e.g. close a tab/panel). */
  onDeleted?: () => void;
};

export type SessionActionFeedback = "toast" | "inline";

export type RemoveSessionOptions = {
  feedback?: SessionActionFeedback;
};

type WsActionOptions = {
  timeout?: number;
  feedback?: SessionActionFeedback;
  requestOverride?: () => Promise<unknown>;
};

type WsActionFn = (
  action: string,
  label: string,
  payload: Record<string, unknown>,
  options?: WsActionOptions,
) => Promise<boolean>;

/** Reports a request outcome and returns false when the operation cannot succeed. */
function useWsAction(): WsActionFn {
  const { toast, updateToast } = useToast();
  const { t } = useTranslation("task");
  return useCallback(
    async (
      action,
      labelKey,
      payload,
      { timeout = 15000, feedback = "toast", requestOverride }: WsActionOptions = {},
    ) => {
      const client = getWebSocketClient();
      if (!client && !requestOverride) return false;
      const toastId =
        feedback === "toast"
          ? toast({
              title: t("task:sessionActionProgress", { action: t(labelKey) }),
              variant: "loading",
            })
          : null;
      try {
        await (requestOverride ? requestOverride() : client!.request(action, payload, timeout));
        if (toastId) {
          updateToast(toastId, {
            title: t("task:sessionActionSuccessful", { action: t(labelKey) }),
            variant: "success",
          });
        }
        return true;
      } catch (error) {
        const msg = error instanceof Error ? error.message : t("common:unknownError");
        const title = t("task:sessionActionFailed", { action: t(labelKey) });
        if (toastId) {
          updateToast(toastId, { title, description: msg, variant: "error" });
        } else {
          toast({ title, description: msg, variant: "error" });
        }
        return false;
      }
    },
    [t, toast, updateToast],
  );
}

/**
 * Binds lifecycle actions to the current task and session identities.
 * Stop, Resume, and Delete reject activation while Restart owns the target.
 * Successful deletion updates selection before removing local session data,
 * then invokes `onDeleted` for caller-owned UI cleanup.
 */
export function useSessionActions({ sessionId, taskId, onDeleted }: SessionActionsArgs) {
  const wsAction = useWsAction();
  const removeTaskSession = useAppStore((state) => state.removeTaskSession);
  const removeQuickChatSession = useAppStore((state) => state.removeQuickChatSession);
  const appStoreApi = useAppStoreApi();
  const { restart, isRestarting, isRestartPending } = useSessionRestart(sessionId, taskId);

  const setPrimary = useCallback(
    () =>
      sessionId &&
      wsAction(
        "session.set_primary",
        "task:sessionActionSetPrimary",
        { session_id: sessionId },
        { timeout: 15000, feedback: "inline" },
      ),
    [sessionId, wsAction],
  );

  const stop = useCallback(
    () =>
      sessionId &&
      !isRestartPending() &&
      wsAction("session.stop", "task:sessionActionStop", { session_id: sessionId }),
    [sessionId, wsAction, isRestartPending],
  );

  const resume = useCallback(
    () =>
      sessionId &&
      taskId &&
      !isRestartPending() &&
      wsAction(
        "session.launch",
        "task:sessionActionResume",
        { task_id: taskId, intent: "resume", session_id: sessionId },
        { timeout: 30000 },
      ),
    [sessionId, taskId, wsAction, isRestartPending],
  );

  const remove = useCallback(
    async (options: RemoveSessionOptions = {}) => {
      if (!sessionId || !taskId || isRestartPending()) return false;
      const deletionTarget = resolveSessionDeletionTarget(
        sessionId,
        taskId,
        appStoreApi.getState().quickChat.sessions,
      );
      const ok = await wsAction(
        "session.delete",
        "task:sessionActionDelete",
        { session_id: sessionId },
        {
          timeout: 15000,
          feedback: options.feedback,
          requestOverride:
            deletionTarget.kind === "quick-chat-task"
              ? () => deleteTaskAfterUserAction(deletionTarget.taskId)
              : undefined,
        },
      );
      if (!ok) return false;

      // Switch the active session BEFORE removing from the store so callers
      // observing activeSessionId don't briefly point at a deleted session.
      const state = appStoreApi.getState();
      if (state.tasks.activeSessionId === sessionId) {
        const sessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
        const remaining = sessions
          .filter((s) => s.id !== sessionId)
          .sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime());
        if (remaining.length > 0) {
          state.setActiveSessionAuto(taskId, remaining[0].id);
        } else {
          state.clearActiveSession();
        }
      }

      removeTaskSession(taskId, sessionId);
      removeQuickChatSession(sessionId);
      onDeleted?.();
      return true;
    },
    [
      sessionId,
      taskId,
      wsAction,
      removeTaskSession,
      removeQuickChatSession,
      appStoreApi,
      onDeleted,
      isRestartPending,
    ],
  );

  return { setPrimary, stop, resume, remove, restart, isRestarting };
}
