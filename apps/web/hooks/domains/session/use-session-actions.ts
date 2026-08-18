"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { deleteTask } from "@/lib/api/domains/kanban-api";
import { resolveSessionDeletionTarget } from "@/lib/session/session-deletion";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskSessionState } from "@/lib/types/http";

export function isSessionStoppable(s: TaskSessionState): boolean {
  return s === "RUNNING" || s === "STARTING" || s === "WAITING_FOR_INPUT";
}
export function isSessionDeletable(s: TaskSessionState): boolean {
  return s !== "RUNNING" && s !== "STARTING";
}
export function isSessionResumable(s: TaskSessionState): boolean {
  return s === "COMPLETED" || s === "FAILED" || s === "CANCELLED";
}

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
 * Shared lifecycle actions for a session (set-primary, stop, resume, delete).
 * Handles backend coordination + local store cleanup. Caller can pass
 * `onDeleted` to perform UI-specific teardown (e.g. dockview panel removal).
 */
export function useSessionActions({ sessionId, taskId, onDeleted }: SessionActionsArgs) {
  const wsAction = useWsAction();
  const removeTaskSession = useAppStore((state) => state.removeTaskSession);
  const removeQuickChatSession = useAppStore((state) => state.removeQuickChatSession);
  const appStoreApi = useAppStoreApi();

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
      sessionId && wsAction("session.stop", "task:sessionActionStop", { session_id: sessionId }),
    [sessionId, wsAction],
  );

  const resume = useCallback(
    () =>
      sessionId &&
      taskId &&
      wsAction(
        "session.launch",
        "task:sessionActionResume",
        { task_id: taskId, intent: "resume", session_id: sessionId },
        { timeout: 30000 },
      ),
    [sessionId, taskId, wsAction],
  );

  const remove = useCallback(
    async (options: RemoveSessionOptions = {}) => {
      if (!sessionId || !taskId) return false;
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
              ? () => deleteTask(deletionTarget.taskId)
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
    ],
  );

  return { setPrimary, stop, resume, remove };
}
