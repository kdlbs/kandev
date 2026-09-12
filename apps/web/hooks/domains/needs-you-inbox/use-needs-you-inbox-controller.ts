import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { getWebSocketClient } from "@/lib/ws/connection";
import { listClarificationInbox } from "@/lib/api/domains/clarification-inbox-api";
import { selectNeedsYouInboxNextSnoozeExpiry } from "@/lib/state/slices/needs-you-inbox/selectors";

// "At most once every 60 seconds" (design-02#Control-flow): the residual
// catch-all for exits none of the other four triggers observes.
const PERIODIC_REFRESH_MS = 60_000;
// A skewed client clock asking early must not spin; floor the reschedule.
const MIN_SNOOZE_RESCHEDULE_MS = 5_000;

/**
 * Owns every Needs-you Inbox refresh trigger (design-02#Control-flow) and the
 * AC .41 snooze-expiry timer. Mounted once, unconditionally of route, so the
 * badge stays right whether or not the Inbox is open (AC .34). The Inbox
 * route component subscribes to the same slice and adds no triggers of its
 * own -- it may call `refresh` again on mount, which is a harmless extra
 * bounded read.
 */
export function useNeedsYouInboxController() {
  const enabled = useFeature("needsYouInbox");
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const connectionStatus = useAppStore((s) => s.connection.status);
  const nextSnoozeExpiry = useAppStore(selectNeedsYouInboxNextSnoozeExpiry);
  const refreshTick = useAppStore((s) => s.needsYouInbox.refreshTick);
  const storeApi = useAppStoreApi();

  const refresh = useCallback(
    async (targetWorkspaceId: string) => {
      const { beginNeedsYouInboxRead, setNeedsYouInboxPage, setNeedsYouInboxError } =
        storeApi.getState();
      const generation = beginNeedsYouInboxRead(targetWorkspaceId);
      try {
        const page = await listClarificationInbox(targetWorkspaceId);
        setNeedsYouInboxPage(targetWorkspaceId, generation, {
          bundles: page.bundles,
          count: page.count,
          hiddenCount: page.hidden_count,
          nextSnoozeExpiry: page.next_snooze_expiry,
          hasMore: page.next_cursor !== undefined,
        });
      } catch {
        setNeedsYouInboxError(targetWorkspaceId, generation);
      }
    },
    [storeApi],
  );

  // Trigger: route mount / workspace change is handled by whichever component
  // reads the active workspace first landing here too, since this hook itself
  // re-reads the moment `workspaceId` changes.
  useEffect(() => {
    if (!enabled || !workspaceId) return;
    void refresh(workspaceId);
  }, [enabled, workspaceId, refresh]);

  // Trigger: WS action-based (session.pending_action_changed,
  // session.state_changed) and edge-detected WS (re)connect. Both live in one
  // effect keyed on connectionStatus so a client created after this hook
  // mounts is picked up the moment status first changes.
  const wasConnectedRef = useRef(false);
  useEffect(() => {
    if (!enabled) return;
    const isConnected = connectionStatus === "connected";
    if (isConnected && !wasConnectedRef.current && workspaceId) {
      void refresh(workspaceId);
    }
    wasConnectedRef.current = isConnected;

    const client = getWebSocketClient();
    if (!client) return;
    const bump = () => storeApi.getState().bumpNeedsYouInboxRefreshTick();
    const offPending = client.on("session.pending_action_changed", bump);
    const offStateChanged = client.on("session.state_changed", bump);
    return () => {
      offPending();
      offStateChanged();
    };
  }, [enabled, connectionStatus, workspaceId, refresh, storeApi]);

  // Applies the WS-triggered refresh tick bumped above.
  const lastTickRef = useRef(refreshTick);
  useEffect(() => {
    if (lastTickRef.current === refreshTick) return;
    lastTickRef.current = refreshTick;
    if (enabled && workspaceId) void refresh(workspaceId);
  }, [refreshTick, enabled, workspaceId, refresh]);

  // Trigger: tab visibility regained (coalesced with focus/pageshow/online).
  useForegroundRefresh(
    () => {
      if (workspaceId) void refresh(workspaceId);
    },
    enabled && !!workspaceId,
    workspaceId,
  );

  // Trigger: bounded periodic re-read, only while the tab is visible.
  useEffect(() => {
    if (!enabled || !workspaceId) return;
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") void refresh(workspaceId);
    }, PERIODIC_REFRESH_MS);
    return () => window.clearInterval(interval);
  }, [enabled, workspaceId, refresh]);

  // Snooze-expiry timer (AC .24/.41): cancelled and re-armed whenever the
  // workspace or the carried expiry changes, so it never fires for a
  // workspace the operator has left.
  useEffect(() => {
    if (!enabled || !workspaceId || !nextSnoozeExpiry) return;
    const delay = Math.max(
      new Date(nextSnoozeExpiry).getTime() - Date.now(),
      MIN_SNOOZE_RESCHEDULE_MS,
    );
    const timeout = window.setTimeout(() => void refresh(workspaceId), delay);
    return () => window.clearTimeout(timeout);
  }, [enabled, workspaceId, nextSnoozeExpiry, refresh]);

  return refresh;
}
