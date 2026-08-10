import { useCallback, useEffect, useState } from "react";
import { listForgejoQueue } from "@/lib/api/domains/forgejo-api";
import type { ForgejoIssue, ForgejoPullRequest, ForgejoRepository } from "@/lib/types/forgejo";

export type ForgejoQueue = {
  issues: { repository: ForgejoRepository; issue: ForgejoIssue }[];
  pull_requests: { repository: ForgejoRepository; pull_request: ForgejoPullRequest }[];
};

export function useForgejoQueue(workspaceId: string | undefined) {
  const [queue, setQueue] = useState<ForgejoQueue | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!workspaceId) return;
    setLoading(true);
    setError(null);
    try {
      setQueue(await listForgejoQueue({ workspaceId }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load Forgejo queue");
    } finally {
      setLoading(false);
    }
  }, [workspaceId]);

  useEffect(() => { void refresh(); }, [refresh]);
  return { queue, loading, error, refresh };
}
