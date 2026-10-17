"use client";

import { useCallback } from "react";
import { useStore } from "zustand";
import { createStore } from "zustand/vanilla";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { isSessionStoppable } from "./session-action-state";

type PendingStore = ReturnType<typeof createPendingStore>;
const pendingByOwner = new WeakMap<object, PendingStore>();

function createPendingStore() {
  return createStore<ReadonlySet<string>>(() => new Set());
}

function getPendingStore(owner: object) {
  let store = pendingByOwner.get(owner);
  if (!store) {
    store = createPendingStore();
    pendingByOwner.set(owner, store);
  }
  return store;
}

export function useSessionRestart(sessionId: string | null | undefined, taskId: string | null) {
  const owner = useAppStoreApi();
  const pending = getPendingStore(owner);
  const isRestarting = useStore(pending, (sessions) => !!sessionId && sessions.has(sessionId));
  const isRestartPending = useCallback(
    () => !!sessionId && pending.getState().has(sessionId),
    [pending, sessionId],
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
      const next = new Set(pending.getState());
      next.delete(sessionId);
      pending.setState(next, true);
    }
  }, [isRestartPending, owner, pending, sessionId, taskId, t, toast, updateToast]);

  return { restart, isRestarting, isRestartPending };
}
