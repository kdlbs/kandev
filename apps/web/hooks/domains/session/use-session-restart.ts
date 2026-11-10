"use client";

import { useCallback } from "react";
import { useStore } from "zustand";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import {
  getSessionRestartStore,
  isSessionRestartPending,
} from "@/lib/session/session-restart-state";
import { isSessionStoppable } from "./session-action-state";

/** Subscribes to the target's pending entry in the current application store. */
export function useSessionRestarting(sessionId: string | null | undefined) {
  const owner = useAppStoreApi();
  return useStore(
    getSessionRestartStore(owner),
    (sessions) => !!sessionId && sessions.has(sessionId),
  );
}

/** Restores selection ownership only when no later user navigation supersedes it. */
function restoreRestartSelection(
  owner: ReturnType<typeof useAppStoreApi>,
  taskId: string,
  sessionId: string,
  navigationRevision: number,
  wasPinned: boolean,
) {
  const current = owner.getState();
  if (
    current.tasks.activeTaskId === taskId &&
    current.tasks.activeSessionId !== sessionId &&
    current.taskRemoval.navigationRevision === navigationRevision
  ) {
    if (wasPinned) current.setActiveSession(taskId, sessionId);
    else current.setActiveSessionAuto(taskId, sessionId);
  }
}

/**
 * Stops and resumes the captured target under one store-owned pending entry.
 * The entry survives consumer unmounts and is released after either request fails
 * or both requests settle. Request success does not imply agent readiness.
 */
export function useSessionRestart(sessionId: string | null | undefined, taskId: string | null) {
  const owner = useAppStoreApi();
  const pending = getSessionRestartStore(owner);
  const isRestarting = useSessionRestarting(sessionId);
  const isRestartPending = useCallback(
    () => isSessionRestartPending(owner, sessionId),
    [owner, sessionId],
  );
  const { t } = useTranslation();
  const { toast, updateToast } = useToast();

  const restart = useCallback(async () => {
    if (isRestartPending()) return false;
    const session = taskId
      ? owner.getState().taskSessionsByTask.itemsByTaskId[taskId]?.find((s) => s.id === sessionId)
      : undefined;
    if (!sessionId || !taskId || !session || !isSessionStoppable(session.state)) {
      toast({
        title: t("task:sessionRestartFailed"),
        description: t("task:sessionRestartUnavailable"),
        variant: "error",
      });
      return false;
    }

    const initial = owner.getState();
    const wasSelected =
      initial.tasks.activeTaskId === taskId && initial.tasks.activeSessionId === sessionId;
    const navigationRevision = initial.taskRemoval.navigationRevision;
    const wasPinned = initial.tasks.pinnedSessionId === sessionId;
    pending.setState(new Set([...pending.getState(), sessionId]), true);
    const toastId = toast({ title: t("task:sessionActionRestart"), variant: "loading" });
    try {
      const client = getWebSocketClient();
      if (!client) throw new Error(t("task:sessionRestartConnectionUnavailable"));
      await client.request("session.stop", { session_id: sessionId }, 15000);
      await client.request(
        "session.launch",
        { task_id: taskId, intent: "resume", session_id: sessionId },
        30000,
      );
      updateToast(toastId, { title: t("task:sessionRestartRequested"), variant: "success" });
      return true;
    } catch (error) {
      updateToast(toastId, {
        title: t("task:sessionRestartFailed"),
        description: error instanceof Error ? error.message : t("common:unknownError"),
        variant: "error",
      });
      return false;
    } finally {
      if (wasSelected)
        restoreRestartSelection(owner, taskId, sessionId, navigationRevision, wasPinned);
      const next = new Set(pending.getState());
      next.delete(sessionId);
      pending.setState(next, true);
    }
  }, [isRestartPending, owner, pending, sessionId, taskId, t, toast, updateToast]);

  return { restart, isRestarting, isRestartPending };
}
