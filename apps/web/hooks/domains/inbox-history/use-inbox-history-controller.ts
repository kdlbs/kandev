import { useCallback, useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { listInboxHistory } from "@/lib/api/domains/inbox-history-api";

export type InboxHistoryController = {
  refresh: (targetWorkspaceId: string) => Promise<void>;
  loadMore: (targetWorkspaceId: string) => Promise<void>;
};

/**
 * Owns the History tab's single read: a point-in-time projection. Unlike the
 * Needs-you controller, this hook subscribes to no WebSocket event and
 * re-reads only on mount, workspace change, and the browser regaining
 * foreground, or an explicit page request -- never on a server-pushed signal.
 */
export function useInboxHistoryController(): InboxHistoryController {
  const enabled = useFeature("needsYouInbox");
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const storeApi = useAppStoreApi();

  const refresh = useCallback(
    async (targetWorkspaceId: string) => {
      const { beginInboxHistoryRead, setInboxHistoryPage, setInboxHistoryError } =
        storeApi.getState();
      const generation = beginInboxHistoryRead(targetWorkspaceId);
      try {
        const page = await listInboxHistory(targetWorkspaceId);
        setInboxHistoryPage(targetWorkspaceId, generation, {
          bundles: page.bundles,
          total: page.total,
          hasMore: page.next_cursor !== undefined,
          nextCursor: page.next_cursor,
        });
      } catch {
        setInboxHistoryError(targetWorkspaceId, generation);
      }
    },
    [storeApi],
  );

  const loadMore = useCallback(
    async (targetWorkspaceId: string) => {
      const state = storeApi.getState();
      const workspace = state.inboxHistory.byWorkspaceId[targetWorkspaceId];
      const generation = state.inboxHistory.generationByWorkspaceId[targetWorkspaceId] ?? 0;
      if (!workspace?.nextCursor || !workspace.hasMore || workspace.status !== "ready") return;
      if (!state.beginInboxHistoryLoadMore(targetWorkspaceId, generation)) return;

      try {
        const page = await listInboxHistory(targetWorkspaceId, { cursor: workspace.nextCursor });
        storeApi.getState().appendInboxHistoryPage(targetWorkspaceId, generation, {
          bundles: page.bundles,
          total: page.total,
          hasMore: page.next_cursor !== undefined,
          nextCursor: page.next_cursor,
        });
      } catch {
        storeApi.getState().setInboxHistoryLoadMoreError(targetWorkspaceId, generation);
      }
    },
    [storeApi],
  );

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    void refresh(workspaceId);
  }, [enabled, workspaceId, refresh]);

  useForegroundRefresh(
    () => {
      if (workspaceId) void refresh(workspaceId);
    },
    enabled && !!workspaceId,
    workspaceId,
  );

  return { refresh, loadMore };
}
