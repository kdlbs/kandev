import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { activeConfigChatOperations } from "./config-chat-operations";
import { t } from "@/lib/i18n";
import { performConfigChatRestart, reconcileConfigChatRestart } from "./config-chat-recovery";

export function useConfigChatRestart(workspaceId: string) {
  const store = useAppStoreApi();
  const restart = useAppStore((state) => state.quickChat.configChatRestarts[workspaceId]);
  const [error, setError] = useState<{ workspaceId: string; message: string } | null>(null);
  const owner = useRef({ workspaceId, revision: 0, mounted: true });
  useEffect(() => {
    owner.current.workspaceId = workspaceId;
    owner.current.mounted = true;
    return () => {
      owner.current.mounted = false;
      owner.current.revision += 1;
    };
  }, [workspaceId]);
  const resetRestart = useCallback(() => {
    owner.current.revision += 1;
    setError(null);
  }, []);

  const run = useCallback(
    async (operation: () => Promise<string | null>) => {
      if (activeConfigChatOperations.has(workspaceId)) return;
      const token = Symbol(workspaceId);
      const revision = owner.current.revision;
      activeConfigChatOperations.set(workspaceId, token);
      setError(null);
      try {
        const nextError = await operation();
        if (
          owner.current.mounted &&
          owner.current.workspaceId === workspaceId &&
          owner.current.revision === revision
        )
          setError(nextError ? { workspaceId, message: nextError } : null);
      } finally {
        if (activeConfigChatOperations.get(workspaceId) === token)
          activeConfigChatOperations.delete(workspaceId);
      }
    },
    [workspaceId],
  );

  const restartSession = useCallback(
    async (session: QuickChatSession) => {
      if (
        !session.taskId ||
        session.workspaceId !== workspaceId ||
        session.kind !== "config" ||
        isQuickChatSetupSessionId(session.sessionId) ||
        store.getState().quickChat.configChatRestarts[workspaceId]
      )
        return;
      await run(() => performConfigChatRestart(store, workspaceId, session));
    },
    [workspaceId, store, run],
  );

  const refreshRestart = useCallback(async () => {
    const current = store.getState().quickChat.configChatRestarts[workspaceId];
    if (!current || current.status !== "uncertain") return;
    await run(() => {
      store.getState().setConfigChatRestart(workspaceId, { ...current, status: "restarting" });
      return reconcileConfigChatRestart(store, workspaceId, current.sessionId);
    });
  }, [workspaceId, store, run]);

  const localError = error?.workspaceId === workspaceId ? error.message : null;
  return {
    restartSession,
    refreshRestart,
    resetRestart,
    isRestarting: restart?.status === "restarting",
    restartBlocked: !!restart,
    restartError:
      restart?.status === "uncertain"
        ? (restart.error ?? t("configChat:restartUncertain"))
        : localError,
  };
}
