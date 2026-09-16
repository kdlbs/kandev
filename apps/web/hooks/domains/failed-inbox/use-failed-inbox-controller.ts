import { useCallback, useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { listFailedInbox } from "@/lib/api/domains/failed-inbox-api";
import type { InboxTab } from "@/lib/failed-inbox/inbox-tab";

// A maximum staleness, not a rate limit (design-01#Control-flow): the
// residual catch-all for an exit that emits no event.
const PERIODIC_REFRESH_MS = 60_000;

/**
 * Owns every Failed-bucket refresh trigger (design-01#Control-flow): Inbox
 * mount, tab selection changing, active workspace changing, the browser tab
 * returning to visible, and a 60s periodic re-read while visible. Every
 * trigger fires regardless of which tab is currently selected. Deliberately
 * NOT app-wide and NOT WS-subscribed: mounted only with the Inbox page,
 * because nothing outside the Inbox renders this bucket's count, and it must
 * not share the Needs-you slice's WS-triggered refresh machinery
 * (design-01#Control-flow, "why it does not stop when Needs you is
 * selected").
 */
export function useFailedInboxController(selectedTab: InboxTab) {
  const enabled = useFeature("needsYouInbox");
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const storeApi = useAppStoreApi();

  const refresh = useCallback(
    async (targetWorkspaceId: string) => {
      const { beginFailedInboxRead, setFailedInboxPage, setFailedInboxError } = storeApi.getState();
      const generation = beginFailedInboxRead(targetWorkspaceId);
      try {
        const page = await listFailedInbox(targetWorkspaceId);
        setFailedInboxPage(targetWorkspaceId, generation, {
          rows: page.rows,
          count: page.count,
          truncated: page.truncated,
        });
      } catch {
        setFailedInboxError(targetWorkspaceId, generation);
      }
    },
    [storeApi],
  );

  // Triggers: Inbox mount, tab selection changing, active workspace changing.
  // One effect covers all three -- it fires on mount and again whenever
  // either dependency changes, regardless of which tab ends up selected.
  useEffect(() => {
    if (!enabled || !workspaceId) return;
    void refresh(workspaceId);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- selectedTab is
    // a trigger, not a read; re-running this effect is the only thing it does.
  }, [enabled, workspaceId, selectedTab, refresh]);

  // Trigger: tab visibility regained (coalesced with focus/pageshow/online).
  useForegroundRefresh(
    () => {
      if (workspaceId) void refresh(workspaceId);
    },
    enabled && !!workspaceId,
    workspaceId,
  );

  // Trigger: bounded periodic re-read, only while the browser tab is visible.
  useEffect(() => {
    if (!enabled || !workspaceId) return;
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") void refresh(workspaceId);
    }, PERIODIC_REFRESH_MS);
    return () => window.clearInterval(interval);
  }, [enabled, workspaceId, refresh]);
}
