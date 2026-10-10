"use client";

import { useEffect, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { isBrowserForeground } from "./git-status-refresh-recovery-utils";
import {
  monitorGitStatusDetails,
  requestGitStatusRefresh,
  retainGitRefreshScope,
  scheduleReplayIfDetailsPending,
} from "./git-status-refresh-coordinator";

/** Reuses the focused session stream for one finite, environment-scoped refresh attempt. */
export function useSessionGitRefresh(sessionId: string | null | undefined, active: boolean): void {
  const store = useAppStoreApi();
  const [foreground, setForeground] = useState(isBrowserForeground);
  const context = useAppStore(
    useShallow((state) => ({
      environmentId: sessionId ? (state.environmentIdBySessionId[sessionId] ?? sessionId) : null,
      connectionStatus: state.connection.status,
    })),
  );

  useEffect(() => {
    const onFocus = () => setForeground(isBrowserForeground());
    const onBlur = () => setForeground(false);
    const onVisibilityChange = () => setForeground(isBrowserForeground());

    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("focus", onFocus);
    window.addEventListener("blur", onBlur);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("focus", onFocus);
      window.removeEventListener("blur", onBlur);
    };
  }, []);

  useEffect(() => {
    const { environmentId, connectionStatus } = context;
    if (!active || !foreground || !sessionId || !environmentId || connectionStatus !== "connected")
      return;
    const client = getWebSocketClient();
    if (!client || client.getStatus() !== "connected") return;

    const release = retainGitRefreshScope(client, environmentId, { sessionId, store });
    const stopMonitoring = monitorGitStatusDetails(client, store, environmentId, sessionId);
    void requestGitStatusRefresh(client, store, sessionId, environmentId).catch(() => undefined);
    scheduleReplayIfDetailsPending(client, store, sessionId, environmentId);
    return () => {
      stopMonitoring();
      release();
    };
  }, [active, context, foreground, sessionId, store]);
}
