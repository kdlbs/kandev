"use client";

import { useCallback } from "react";
import type { TFunction } from "i18next";
import { useStore } from "zustand";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import {
  getSessionRestartStore,
  isSessionRestartPending,
} from "@/lib/session/session-restart-state";
import type { TaskSession } from "@/lib/types/http";
import {
  requestSessionRestartReview,
  type SessionRestartDecision,
} from "@/lib/session/session-restart-review";
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

const STALE_REVIEW = Symbol();

async function reviewSandboxRestart(
  owner: ReturnType<typeof useAppStoreApi>,
  taskId: string,
  session: TaskSession,
): Promise<SessionRestartDecision | null> {
  const attemptId = session.execution_snapshot!.sandbox!.attempt_id;
  const identity = JSON.stringify(owner.getState().auth);
  const decision = await requestSessionRestartReview(owner, {
    taskId,
    sessionId: session.id,
    attemptId,
    identity,
    navigationRevision: owner.getState().taskRemoval.navigationRevision,
  });
  if (!decision) return null;
  const current = owner.getState();
  const target = current.taskSessionsByTask.itemsByTaskId[taskId]?.find((s) => s.id === session.id);
  if (
    JSON.stringify(current.auth) !== identity ||
    target?.execution_snapshot?.sandbox?.attempt_id !== attemptId
  ) {
    throw STALE_REVIEW;
  }
  if (
    decision.mode === "current" &&
    (current.auth.mode !== "enabled" ||
      !current.auth.authenticated ||
      current.auth.user?.role !== "admin")
  ) {
    throw STALE_REVIEW;
  }
  return decision;
}

function restartFailureDescription(error: unknown, t: TFunction) {
  if (error === STALE_REVIEW) return t("agents:restartReviewStale");
  if (error instanceof Error) return error.message;
  return t("common:unknownError");
}

async function dispatchRestart(
  taskId: string,
  session: TaskSession,
  decision: SessionRestartDecision,
  t: TFunction,
) {
  const sessionId = session.id;
  const client = getWebSocketClient();
  if (!client) throw new Error(t("task:sessionRestartConnectionUnavailable"));
  await client.request("session.stop", { session_id: sessionId }, 15000);
  await client.request(
    "session.launch",
    {
      task_id: taskId,
      intent: "resume",
      session_id: sessionId,
      ...(decision.mode === "current"
        ? {
            sandbox_restart_review: {
              attempt_id: session.execution_snapshot!.sandbox!.attempt_id,
              selection_digest: decision.selectionDigest,
            },
          }
        : {}),
    },
    30000,
  );
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
    let toastId: ReturnType<typeof toast> | null = null;
    try {
      const decision = session.execution_snapshot?.sandbox
        ? await reviewSandboxRestart(owner, taskId, session)
        : { mode: "captured" as const };
      if (!decision) return false;
      toastId = toast({ title: t("task:sessionActionRestart"), variant: "loading" });
      await dispatchRestart(taskId, session, decision, t);
      updateToast(toastId, { title: t("task:sessionRestartRequested"), variant: "success" });
      return true;
    } catch (error) {
      const failure = {
        title: t("task:sessionRestartFailed"),
        description: restartFailureDescription(error, t),
        variant: "error" as const,
      };
      if (toastId) updateToast(toastId, failure);
      else toast(failure);
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
