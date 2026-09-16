import { useCallback, useEffect, useLayoutEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { FOREGROUND_EVENT_COALESCE_MS, useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { getWebSocketClient } from "@/lib/ws/connection";
import { listClarificationInbox } from "@/lib/api/domains/clarification-inbox-api";
import { selectNeedsYouInboxNextSnoozeExpiry } from "@/lib/state/slices/needs-you-inbox/selectors";
import { readBootPayload } from "@/src/boot-payload";

// "At most once every 60 seconds" (design-02#Control-flow): the residual
// catch-all for exits none of the other four triggers observes.
const PERIODIC_REFRESH_MS = 60_000;
// A skewed client clock asking early must not spin; floor the reschedule.
const MIN_SNOOZE_RESCHEDULE_MS = 5_000;
// `next_snooze_expiry` is serialized at whole-second RFC3339 precision while
// the stored expiry can carry subsecond precision, so firing exactly at the
// reported instant can land up to a second before the row is actually
// eligible: the re-read then reports the same (still-future) expiry string,
// and an unchanged effect dependency never rearms the timeout. Firing a
// second late instead guarantees the read crosses the real boundary.
const SNOOZE_RESCHEDULE_BUFFER_MS = 1_000;

// Applies the boot-hydration producer (needs-you-inbox
// design-01#Data-and-contracts) via seedNeedsYouInboxBoot, so the badge
// carries a value before the live read below ever resolves. A
// useLayoutEffect, not useEffect, so it always runs before this hook's own
// refresh-trigger effects in the same commit -- seedNeedsYouInboxBoot's own
// generation-0 guard makes the ordering a belt-and-suspenders correctness
// property rather than the only thing preventing a stale overwrite, but boot
// data seeding after the first live read started would otherwise waste the
// one chance it has to avoid the pre-read flash.
function useNeedsYouInboxBootSeed(
  enabled: boolean,
  workspaceId: string | null,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  const seededRef = useRef(false);
  useLayoutEffect(() => {
    if (!enabled || !workspaceId || seededRef.current) return;
    seededRef.current = true;
    const boot = readBootPayload().initialState?.needsYouInboxBoot;
    if (!boot || boot.workspaceId !== workspaceId) return;
    storeApi.getState().seedNeedsYouInboxBoot(workspaceId, {
      count: boot.count,
      hasMore: boot.hasMore,
      nextSnoozeExpiry: boot.nextSnoozeExpiry,
    });
  }, [enabled, workspaceId, storeApi]);
}

// Trigger: WS action-based (session.pending_action_changed,
// session.state_changed) and edge-detected WS (re)connect. Both live in one
// effect keyed on connectionStatus so a client created after this hook mounts
// is picked up the moment status first changes.
function useNeedsYouInboxWsRefresh(
  enabled: boolean,
  workspaceId: string | null,
  connectionStatus: string,
  refresh: (targetWorkspaceId: string) => Promise<void>,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  const wasConnectedRef = useRef(false);
  // session.state_changed is broadcast workspace-wide, so a burst of
  // unrelated session transitions must not cause a burst of reads: coalesce
  // with the same window use-foreground-refresh.ts uses for its own bursty
  // browser events. Unlike that hook's duplicate browser events, though,
  // session.state_changed and session.pending_action_changed are distinct
  // signals from distinct sessions -- a read triggered by one cannot reflect
  // a change the other carries -- so an event landing inside the window is
  // queued for one trailing bump at the end of it, never dropped.
  const lastWsBumpAtRef = useRef(-Infinity);
  // Held in a ref alongside `lastWsBumpAtRef`, not effect-local: this effect
  // re-runs on every `connectionStatus` change (e.g. a reconnect), and its own
  // cleanup only tears down the WS listeners it just registered -- clearing a
  // still-pending trailing bump there would drop it silently on a re-run,
  // exactly the "dropped, not deferred" bug this coalescing exists to avoid.
  // Only a true unmount (below) clears it.
  const trailingBumpTimeoutRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (!enabled) return;
    const isConnected = connectionStatus === "connected";
    if (isConnected && !wasConnectedRef.current && workspaceId) {
      void refresh(workspaceId);
    }
    wasConnectedRef.current = isConnected;

    const client = getWebSocketClient();
    if (!client) return;
    const doBump = () => {
      lastWsBumpAtRef.current = Date.now();
      storeApi.getState().bumpNeedsYouInboxRefreshTick();
    };
    const bump = () => {
      const elapsed = Date.now() - lastWsBumpAtRef.current;
      if (elapsed >= FOREGROUND_EVENT_COALESCE_MS) {
        // A pending trailing timer due but not yet fired loses the race to
        // this immediate path; cancel it so the burst yields one read here,
        // not one here and a second when the timer catches up.
        if (trailingBumpTimeoutRef.current !== undefined) {
          window.clearTimeout(trailingBumpTimeoutRef.current);
          trailingBumpTimeoutRef.current = undefined;
        }
        doBump();
        return;
      }
      if (trailingBumpTimeoutRef.current !== undefined) return;
      trailingBumpTimeoutRef.current = window.setTimeout(() => {
        trailingBumpTimeoutRef.current = undefined;
        doBump();
      }, FOREGROUND_EVENT_COALESCE_MS - elapsed);
    };
    const offPending = client.on("session.pending_action_changed", bump);
    const offStateChanged = client.on("session.state_changed", bump);
    return () => {
      offPending();
      offStateChanged();
    };
  }, [enabled, connectionStatus, workspaceId, refresh, storeApi]);

  useEffect(() => {
    return () => {
      if (trailingBumpTimeoutRef.current !== undefined) {
        window.clearTimeout(trailingBumpTimeoutRef.current);
        trailingBumpTimeoutRef.current = undefined;
      }
    };
  }, []);
}

/**
 * Owns every Needs-you Inbox refresh trigger (design-02#Control-flow) and the
 * snooze-expiry timer. Mounted once, unconditionally of route, so the badge
 * stays right whether or not the Inbox is open. The Inbox route component
 * subscribes to the same slice and adds no triggers of its
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

  useNeedsYouInboxBootSeed(enabled, workspaceId, storeApi);

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

  useNeedsYouInboxWsRefresh(enabled, workspaceId, connectionStatus, refresh, storeApi);

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

  // Snooze-expiry timer: cancelled and re-armed whenever the workspace or the
  // carried expiry changes, so it never fires for a workspace the operator
  // has left.
  useEffect(() => {
    if (!enabled || !workspaceId || !nextSnoozeExpiry) return;
    const delay = Math.max(
      new Date(nextSnoozeExpiry).getTime() - Date.now() + SNOOZE_RESCHEDULE_BUFFER_MS,
      MIN_SNOOZE_RESCHEDULE_MS,
    );
    const timeout = window.setTimeout(() => void refresh(workspaceId), delay);
    return () => window.clearTimeout(timeout);
  }, [enabled, workspaceId, nextSnoozeExpiry, refresh]);

  return refresh;
}
