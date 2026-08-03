import { useEffect, useMemo, useRef } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { ConnectionStatus } from "@/lib/types/connection";

export type SessionLiveSyncParams = {
  connectionStatus: ConnectionStatus;
  taskId: string | null;
  sessionIds: readonly string[];
};

export function stableSessionIdsKey(sessionIds: readonly string[]): string {
  return [...new Set(sessionIds.filter(Boolean))].sort().join(",");
}

export function useSessionLiveSyncSubscriptions({
  connectionStatus,
  taskId,
  sessionIds,
}: SessionLiveSyncParams): string[] {
  const membershipKey = useMemo(() => stableSessionIdsKey(sessionIds), [sessionIds]);
  const stableSessionIds = useMemo(
    () => (membershipKey ? membershipKey.split(",") : []),
    [membershipKey],
  );
  const subscriptionsRef = useRef(new Map<string, () => void>());

  useEffect(() => {
    const subscriptions = subscriptionsRef.current;
    const clearSubscriptions = () => {
      for (const unsubscribe of subscriptions.values()) unsubscribe();
      subscriptions.clear();
    };

    if (connectionStatus !== "connected" || !taskId) {
      clearSubscriptions();
      return;
    }

    const client = getWebSocketClient();
    if (!client) {
      clearSubscriptions();
      return;
    }

    const desired = new Set(stableSessionIds);
    for (const [sessionId, unsubscribe] of subscriptions) {
      if (desired.has(sessionId)) continue;
      unsubscribe();
      subscriptions.delete(sessionId);
    }
    for (const sessionId of stableSessionIds) {
      if (subscriptions.has(sessionId)) continue;
      subscriptions.set(sessionId, client.subscribeSession(sessionId));
    }
  }, [connectionStatus, membershipKey, stableSessionIds, taskId]);

  useEffect(
    () => () => {
      for (const unsubscribe of subscriptionsRef.current.values()) unsubscribe();
      subscriptionsRef.current.clear();
    },
    [],
  );

  return stableSessionIds;
}
