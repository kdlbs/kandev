"use client";

import { useCallback, useEffect } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import {
  monitorGitStatusDetails,
  requestGitStatusRefresh,
  retainGitRefreshScope,
  scheduleReplayIfDetailsPending,
} from "./git-status-refresh-coordinator";

/** Reuses the focused session stream for one finite, environment-scoped refresh attempt. */
export function useSessionGitRefresh(
  sessionId: string | null | undefined,
  active: boolean,
): () => void {
  const store = useAppStoreApi();
  const context = useAppStore(
    useShallow((state) => ({
      environmentId: sessionId ? (state.environmentIdBySessionId[sessionId] ?? sessionId) : null,
      connectionStatus: state.connection.status,
    })),
  );

  useEffect(() => {
    const { environmentId, connectionStatus } = context;
    if (!active || !sessionId || !environmentId || connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (!client || client.getStatus() !== "connected") return;

    const release = retainGitRefreshScope(client, environmentId);
    const stopMonitoring = monitorGitStatusDetails(client, store, environmentId);
    void requestGitStatusRefresh(client, store, sessionId, environmentId).catch(() => undefined);
    scheduleReplayIfDetailsPending(client, store, sessionId, environmentId);
    return () => {
      stopMonitoring();
      release();
    };
  }, [active, context, sessionId, store]);

  return useCallback(() => {
    const { environmentId, connectionStatus } = context;
    const client = getWebSocketClient();
    if (
      !sessionId ||
      !environmentId ||
      connectionStatus !== "connected" ||
      !client ||
      client.getStatus() !== "connected"
    ) {
      return;
    }
    void requestGitStatusRefresh(client, store, sessionId, environmentId).catch(() => undefined);
  }, [context, sessionId, store]);
}
