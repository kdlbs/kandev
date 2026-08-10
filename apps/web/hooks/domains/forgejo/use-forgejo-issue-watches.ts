import { useCallback, useEffect } from "react";
import {
  deleteForgejoIssueWatch,
  listForgejoIssueWatches,
  pollForgejoIssueWatch,
  saveForgejoIssueWatch,
} from "@/lib/api/domains/forgejo-api";
import { useAppStore } from "@/components/state-provider";
import type { ForgejoIssueWatch } from "@/lib/types/forgejo";

export function useForgejoIssueWatches(workspaceId: string | undefined) {
  const revision = useAppStore((app) =>
    workspaceId ? (app.forgejoWorkspaceDataRevisions[workspaceId] ?? 0) : 0,
  );
  const state = useAppStore((app) =>
    workspaceId ? app.forgejoIssueWatches[workspaceId] : undefined,
  );
  const setWatches = useAppStore((app) => app.setForgejoIssueWatchesState);
  const setLoading = useAppStore((app) => app.setForgejoIssueWatchesLoading);

  const load = useCallback(async () => {
    if (!workspaceId) return [];
    setLoading(workspaceId, true);
    try {
      const watches = (await listForgejoIssueWatches({ workspaceId })).watches;
      setWatches(workspaceId, watches);
      return watches;
    } catch (cause) {
      setWatches(
        workspaceId,
        [],
        cause instanceof Error ? cause.message : "Could not load Forgejo issue watches",
      );
      return [];
    } finally {
      setLoading(workspaceId, false);
    }
  }, [setLoading, setWatches, workspaceId]);

  useEffect(() => {
    void load();
  }, [load, revision]);

  const save = useCallback(
    async (watch: Partial<ForgejoIssueWatch> & { owner: string; repo: string }) => {
      if (!workspaceId) throw new Error("workspaceId required");
      await saveForgejoIssueWatch(watch, { workspaceId });
      return load();
    },
    [load, workspaceId],
  );
  const remove = useCallback(
    async (watchId: string) => {
      if (!workspaceId) throw new Error("workspaceId required");
      await deleteForgejoIssueWatch(watchId, { workspaceId });
      return load();
    },
    [load, workspaceId],
  );
  const poll = useCallback(
    async (watchId: string) => {
      if (!workspaceId) throw new Error("workspaceId required");
      const result = await pollForgejoIssueWatch(watchId, { workspaceId });
      await load();
      return result;
    },
    [load, workspaceId],
  );

  return {
    watches: state?.data ?? [],
    loading: state?.loading ?? false,
    error: state?.error ?? null,
    load,
    save,
    remove,
    poll,
  };
}
