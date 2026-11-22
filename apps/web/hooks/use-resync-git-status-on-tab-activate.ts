"use client";

import { useEffect } from "react";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { getWebSocketClient } from "@/lib/ws/connection";

/** Request a fresh git snapshot when a portal-hosted panel becomes active. */
export function useResyncGitStatusOnTabActivate(panelId: string, sessionId: string | null) {
  useEffect(() => {
    if (!sessionId) return;
    const entry = panelPortalManager.get(panelId);
    if (!entry?.api) return;

    const refreshNow = () => {
      getWebSocketClient()?.refreshSessionData(sessionId);
    };

    // An already-active panel has no activation transition to emit.
    if (entry.api.isActive) refreshNow();
    const disposable = entry.api.onDidActiveChange((event) => {
      if (event.isActive) refreshNow();
    });
    return () => disposable.dispose();
  }, [panelId, sessionId]);
}
