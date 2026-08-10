import { useCallback, useEffect } from "react";
import { listForgejoQueue } from "@/lib/api/domains/forgejo-api";
import type { ForgejoIssue, ForgejoPullRequest, ForgejoRepository } from "@/lib/types/forgejo";
import { useAppStore } from "@/components/state-provider";

export type ForgejoQueue = {
  issues: { repository: ForgejoRepository; issue: ForgejoIssue }[];
  pull_requests: { repository: ForgejoRepository; pull_request: ForgejoPullRequest }[];
};

export function useForgejoQueue(workspaceId: string | undefined) {
  const state = useAppStore((app) =>
    workspaceId ? app.forgejoQueue[workspaceId] : undefined,
  );
  const setQueue = useAppStore((app) => app.setForgejoQueueState);
  const setLoading = useAppStore((app) => app.setForgejoQueueLoading);

  const refresh = useCallback(async () => {
    if (!workspaceId) return;
    setLoading(workspaceId, true);
    try {
      setQueue(workspaceId, await listForgejoQueue({ workspaceId }));
    } catch (cause) {
      setQueue(
        workspaceId,
        null,
        cause instanceof Error ? cause.message : "Could not load Forgejo queue",
      );
    } finally {
      setLoading(workspaceId, false);
    }
  }, [setLoading, setQueue, workspaceId]);

  useEffect(() => { void refresh(); }, [refresh]);
  return {
    queue: state?.data ?? null,
    loading: state?.loading ?? false,
    error: state?.error ?? null,
    refresh,
  };
}
