"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listQuickChatSessions } from "@/lib/api/domains/workspace-api";
import { getStoredQuickChatNames } from "@/lib/local-storage";
import { toQuickChatSessions } from "@/lib/quick-chat/map-sessions";
import { migrateStoredQuickChatNames } from "@/lib/quick-chat/rename";

/**
 * Keeps this client's quick-chat tabs in step with the server's list.
 *
 * Live changes arrive over the WebSocket, but a client that was asleep or
 * disconnected (a backgrounded phone tab is the common case) misses those
 * events entirely and would otherwise keep showing tabs nobody else has —
 * and keep missing tabs everyone else has — until a full page reload.
 * Re-reading the list whenever the socket (re)connects closes that gap.
 */
export function useQuickChatResync(workspaceId: string | null): void {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const syncQuickChatSessions = useAppStore((state) => state.syncQuickChatSessions);
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  // Resync once per connection, not on every unrelated status re-render.
  const lastSyncedConnection = useRef<string | null>(null);

  useEffect(() => {
    if (connectionStatus !== "connected") {
      lastSyncedConnection.current = null;
      return;
    }
    if (!workspaceId || lastSyncedConnection.current === workspaceId) return;
    lastSyncedConnection.current = workspaceId;

    let cancelled = false;
    listQuickChatSessions(workspaceId)
      .then((response) => {
        if (cancelled) return;
        const sessions = toQuickChatSessions(response.sessions);
        // Store the session rows before the tabs. A tab without its row cannot
        // subscribe or accept input (useSession bails, requireSessionInputMode
        // throws), so a resync-only tab would otherwise render but be dead.
        for (const taskSession of response.task_sessions) {
          setTaskSession(taskSession);
        }
        syncQuickChatSessions(workspaceId, sessions);
        // Renames made before names were stored server-side live only in this
        // browser; push them up once so they reach the user's other devices.
        void migrateStoredQuickChatNames(sessions, getStoredQuickChatNames());
      })
      .catch(() => {
        // A failed resync must not clear the user's tabs; retry on next connect.
        if (!cancelled) lastSyncedConnection.current = null;
      });

    return () => {
      cancelled = true;
    };
  }, [connectionStatus, workspaceId, setTaskSession, syncQuickChatSessions]);
}
