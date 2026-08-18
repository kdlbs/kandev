"use client";

import { useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listQuickChatSessions } from "@/lib/api/domains/workspace-api";
import { listQuickTerminalTabs, toQuickTerminalTab } from "@/lib/api/domains/quick-terminal-api";
import { getStoredQuickChatNames } from "@/lib/local-storage";
import { toQuickChatSessions } from "@/lib/quick-chat/map-sessions";
import { migrateStoredQuickChatNames } from "@/lib/quick-chat/rename";

/** How many times a resync may refetch after observing a newer revision. */
const MAX_RESYNC_RETRIES = 3;
const RESYNC_RETRY_DELAY_MS = 50;

/**
 * Whether a reconnect snapshot is older than the live row. Parses both
 * timestamps so an exact-second value ("...00Z") compares correctly against a
 * newer fractional one ("...00.1Z"); a lexicographic comparison would misorder
 * them. Mirrors isStaleSessionStateEvent without pulling the handler module
 * graph (and its i18n init) into the resync hook.
 */
function isStaleTaskSession(
  live: { updated_at?: string } | undefined,
  snapshotUpdatedAt: string | undefined,
): boolean {
  if (!snapshotUpdatedAt || !live?.updated_at) return false;
  const snapshotTime = Date.parse(snapshotUpdatedAt);
  const liveTime = Date.parse(live.updated_at);
  if (Number.isNaN(snapshotTime) || Number.isNaN(liveTime)) return false;
  return snapshotTime < liveTime;
}

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
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const syncQuickChatSessions = useAppStore((state) => state.syncQuickChatSessions);
  const syncQuickTerminalTabs = useAppStore((state) => state.syncQuickTerminalTabs);
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
    const resync = async () => {
      let retryCount = 0;
      while (!cancelled) {
        const revision = store.getState().quickChat.syncRevisionByWorkspace[workspaceId] ?? 0;
        try {
          const [response, terminalResponse] = await Promise.all([
            listQuickChatSessions(workspaceId),
            listQuickTerminalTabs(workspaceId),
          ]);
          if (cancelled) return;

          const currentRevision =
            store.getState().quickChat.syncRevisionByWorkspace[workspaceId] ?? 0;
          if (currentRevision !== revision) {
            if (retryCount >= MAX_RESYNC_RETRIES) return;
            retryCount += 1;
            await new Promise((resolve) => setTimeout(resolve, RESYNC_RETRY_DELAY_MS));
            continue;
          }

          const sessions = toQuickChatSessions(response.sessions);
          // Store the session rows before the tabs. A tab without its row cannot
          // subscribe or accept input (useSession bails, requireSessionInputMode
          // throws), so a resync-only tab would otherwise render but be dead.
          // Rows older than the live row are skipped: a reconnect snapshot must
          // never regress state a newer WebSocket event already applied.
          for (const taskSession of response.task_sessions) {
            const liveSession = store.getState().taskSessions.items[taskSession.id];
            if (isStaleTaskSession(liveSession, taskSession.updated_at)) continue;
            setTaskSession(taskSession);
          }
          syncQuickChatSessions(workspaceId, sessions);
          syncQuickTerminalTabs(workspaceId, terminalResponse.tabs.map(toQuickTerminalTab));
          // Renames made before names were stored server-side live only in this
          // browser; push them up once so they reach the user's other devices.
          void migrateStoredQuickChatNames(sessions, getStoredQuickChatNames());
          return;
        } catch {
          // A failed resync must not clear the user's tabs; retry on next connect.
          if (!cancelled) lastSyncedConnection.current = null;
          return;
        }
      }
    };
    void resync();

    return () => {
      cancelled = true;
    };
  }, [
    connectionStatus,
    workspaceId,
    store,
    setTaskSession,
    syncQuickChatSessions,
    syncQuickTerminalTabs,
  ]);
}
