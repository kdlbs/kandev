"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
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

type WsActionFn = (
  action: string,
  label: string,
  payload: Record<string, unknown>,
  timeout?: number,
  feedback?: SessionActionFeedback,
) => Promise<boolean>;

function useWsAction(): WsActionFn {
  const { toast, updateToast } = useToast();
  const { t } = useTranslation("task");
  return useCallback(
    async (action, labelKey, payload, timeout = 15000, feedback = "toast") => {
      const client = getWebSocketClient();
      if (!client) return false;
      const toastId =
        feedback === "toast"
          ? toast({
              title: t("task:sessionActionProgress", { action: t(labelKey) }),
              variant: "loading",
            })
          : null;
      try {
        await client.request(action, payload, timeout);
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
  const appStoreApi = useAppStoreApi();

  const setPrimary = useCallback(
    () =>
      sessionId &&
      wsAction(
        "session.set_primary",
        "task:sessionActionSetPrimary",
        { session_id: sessionId },
        15000,
        "inline",
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
        30000,
      ),
    [sessionId, taskId, wsAction],
  );

  const remove = useCallback(
    async (options: RemoveSessionOptions = {}) => {
      if (!sessionId || !taskId) return false;
      const ok = await wsAction(
        "session.delete",
        "task:sessionActionDelete",
        { session_id: sessionId },
        15000,
        options.feedback,
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
      onDeleted?.();
      return true;
    },
    [sessionId, taskId, wsAction, removeTaskSession, appStoreApi, onDeleted],
  );

  return { setPrimary, stop, resume, remove };
}
