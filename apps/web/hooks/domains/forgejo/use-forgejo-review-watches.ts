import { useCallback, useEffect } from "react";
import {
  deleteForgejoReviewWatch,
  listForgejoReviewWatches,
  pollForgejoReviewWatch,
  saveForgejoReviewWatch,
} from "@/lib/api/domains/forgejo-api";
import { useAppStore } from "@/components/state-provider";
import type { ForgejoReviewWatch } from "@/lib/types/forgejo";

export function useForgejoReviewWatches(workspaceId: string | undefined) {
  const state = useAppStore((app) =>
    workspaceId ? app.forgejoReviewWatches[workspaceId] : undefined,
  );
  const setWatches = useAppStore((app) => app.setForgejoReviewWatchesState);
  const setLoading = useAppStore((app) => app.setForgejoReviewWatchesLoading);
  const load = useCallback(async () => {
    if (!workspaceId) return [];
    setLoading(workspaceId, true);
    try {
      const watches = (await listForgejoReviewWatches({ workspaceId })).watches;
      setWatches(workspaceId, watches);
      return watches;
    } catch (cause) {
      setWatches(
        workspaceId,
        [],
        cause instanceof Error ? cause.message : "Could not load Forgejo review watches",
      );
      return [];
    } finally {
      setLoading(workspaceId, false);
    }
  }, [setLoading, setWatches, workspaceId]);
  useEffect(() => {
    void load();
  }, [load]);
  const save = useCallback(
    async (watch: Partial<ForgejoReviewWatch> & { owner: string; repo: string }) => {
      if (!workspaceId) throw new Error("workspaceId required");
      await saveForgejoReviewWatch(watch, { workspaceId });
      return load();
    },
    [load, workspaceId],
  );
  const remove = useCallback(
    async (watchId: string) => {
      if (!workspaceId) throw new Error("workspaceId required");
      await deleteForgejoReviewWatch(watchId, { workspaceId });
      return load();
    },
    [load, workspaceId],
  );
  const poll = useCallback(
    async (watchId: string) => {
      if (!workspaceId) throw new Error("workspaceId required");
      const result = await pollForgejoReviewWatch(watchId, { workspaceId });
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
