import { useCallback, useEffect } from "react";
import {
  deleteForgejoActionPreset,
  listForgejoActionPresets,
  saveForgejoActionPreset,
} from "@/lib/api/domains/forgejo-api";
import { useAppStore } from "@/components/state-provider";
import type { ForgejoActionPreset } from "@/lib/types/forgejo";

export function useForgejoActionPresets(workspaceId: string | undefined) {
  const revision = useAppStore((app) =>
    workspaceId ? (app.forgejoWorkspaceDataRevisions[workspaceId] ?? 0) : 0,
  );
  const state = useAppStore((app) =>
    workspaceId ? app.forgejoActionPresets[workspaceId] : undefined,
  );
  const setPresets = useAppStore((app) => app.setForgejoActionPresetsState);
  const setLoading = useAppStore((app) => app.setForgejoActionPresetsLoading);
  const load = useCallback(async () => {
    if (!workspaceId) return [];
    setLoading(workspaceId, true);
    try {
      const presets = (await listForgejoActionPresets({ workspaceId })).presets;
      setPresets(workspaceId, presets);
      return presets;
    } catch (cause) {
      setPresets(
        workspaceId,
        [],
        cause instanceof Error ? cause.message : "Could not load Forgejo action presets",
      );
      return [];
    } finally {
      setLoading(workspaceId, false);
    }
  }, [setLoading, setPresets, workspaceId]);
  useEffect(() => {
    void load();
  }, [load, revision]);
  const save = useCallback(
    async (preset: Partial<ForgejoActionPreset> & { kind: string; name: string }) => {
      if (!workspaceId) throw new Error("workspaceId required");
      await saveForgejoActionPreset(preset, { workspaceId });
      return load();
    },
    [load, workspaceId],
  );
  const remove = useCallback(
    async (presetId: string) => {
      if (!workspaceId) throw new Error("workspaceId required");
      await deleteForgejoActionPreset(presetId, { workspaceId });
      return load();
    },
    [load, workspaceId],
  );
  return {
    presets: state?.data ?? [],
    loading: state?.loading ?? false,
    error: state?.error ?? null,
    load,
    save,
    remove,
  };
}
