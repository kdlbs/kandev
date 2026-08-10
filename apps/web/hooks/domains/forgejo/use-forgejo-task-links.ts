import { useCallback, useEffect, useState } from "react";
import {
  listForgejoTaskIssues,
  listForgejoTaskPRs,
  refreshForgejoTaskIssue,
  refreshForgejoTaskPullRequest,
  unlinkForgejoIssue,
  unlinkForgejoPullRequest,
} from "@/lib/api/domains/forgejo-api";
import type { ForgejoTaskIssue, ForgejoTaskPR } from "@/lib/types/forgejo";

/** Workspace-scoped task links. The backend is the authority: reload after every mutation. */
export function useForgejoTaskLinks(
  workspaceId: string | null | undefined,
  taskId: string | null | undefined,
) {
  const [issues, setIssues] = useState<ForgejoTaskIssue[]>([]);
  const [pullRequests, setPullRequests] = useState<ForgejoTaskPR[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    if (!workspaceId || !taskId) {
      setIssues([]);
      setPullRequests([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const [issueResult, prResult] = await Promise.all([
        listForgejoTaskIssues(taskId, { workspaceId }),
        listForgejoTaskPRs(taskId, { workspaceId }),
      ]);
      setIssues(issueResult.issues);
      setPullRequests(prResult.pull_requests);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load Forgejo task links");
    } finally {
      setLoading(false);
    }
  }, [taskId, workspaceId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const refreshIssue = useCallback(
    async (linkId: string) => {
      if (!workspaceId) return;
      await refreshForgejoTaskIssue(linkId, { workspaceId });
      await reload();
    },
    [reload, workspaceId],
  );

  const refreshPullRequest = useCallback(
    async (linkId: string) => {
      if (!workspaceId) return;
      await refreshForgejoTaskPullRequest(linkId, { workspaceId });
      await reload();
    },
    [reload, workspaceId],
  );

  const removeIssue = useCallback(
    async (linkId: string) => {
      if (!workspaceId) return;
      await unlinkForgejoIssue(linkId, { workspaceId });
      await reload();
    },
    [reload, workspaceId],
  );

  const removePullRequest = useCallback(
    async (linkId: string) => {
      if (!workspaceId) return;
      await unlinkForgejoPullRequest(linkId, { workspaceId });
      await reload();
    },
    [reload, workspaceId],
  );

  return {
    issues,
    pullRequests,
    loading,
    error,
    reload,
    refreshIssue,
    refreshPullRequest,
    removeIssue,
    removePullRequest,
  };
}
