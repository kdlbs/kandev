import { useCallback, useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { listInboxHistory } from "@/lib/api/domains/inbox-history-api";

/**
 * Owns the History tab's single read. AC .22 makes this a point-in-time
 * projection: unlike the Needs-you controller, this hook subscribes to no
 * WebSocket event and re-reads only on mount, workspace change, and the
 * browser regaining foreground -- never on a server-pushed signal.
 */
export function useInboxHistoryController() {
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
        });
      } catch {
        setInboxHistoryError(targetWorkspaceId, generation);
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

  return refresh;
}
