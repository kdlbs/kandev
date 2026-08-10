import { useCallback, useEffect } from "react";
import {
  deleteForgejoConfig,
  getForgejoConfig,
  refreshForgejoConnection,
  setForgejoConfig,
} from "@/lib/api/domains/forgejo-api";
import { useAppStore } from "@/components/state-provider";
import type { SetForgejoConfigRequest } from "@/lib/types/forgejo";

export function useForgejoConfig(workspaceId: string | undefined) {
  const state = useAppStore((app) => (workspaceId ? app.forgejoConfig[workspaceId] : undefined));
  const setConfig = useAppStore((app) => app.setForgejoConfigState);
  const setLoading = useAppStore((app) => app.setForgejoConfigLoading);
  const reset = useAppStore((app) => app.resetForgejoWorkspaceState);

  const load = useCallback(async () => {
    if (!workspaceId) return null;
    setLoading(workspaceId, true);
    try {
      const config = await getForgejoConfig({ workspaceId });
      setConfig(workspaceId, config);
      return config;
    } catch (cause) {
      setConfig(
        workspaceId,
        null,
        cause instanceof Error ? cause.message : "Could not load Forgejo connection",
      );
      return null;
    } finally {
      setLoading(workspaceId, false);
    }
  }, [setConfig, setLoading, workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = useCallback(
    async (payload: SetForgejoConfigRequest) => {
      if (!workspaceId) throw new Error("workspaceId required");
      const config = await setForgejoConfig(payload, { workspaceId });
      setConfig(workspaceId, config);
      return config;
    },
    [setConfig, workspaceId],
  );

  const refresh = useCallback(async () => {
    if (!workspaceId) throw new Error("workspaceId required");
    const config = await refreshForgejoConnection({ workspaceId });
    setConfig(workspaceId, config);
    return config;
  }, [setConfig, workspaceId]);

  const disconnect = useCallback(async () => {
    if (!workspaceId) throw new Error("workspaceId required");
    await deleteForgejoConfig({ workspaceId });
    reset(workspaceId);
  }, [reset, workspaceId]);

  return {
    config: state?.data ?? null,
    loading: state?.loading ?? false,
    error: state?.error ?? null,
    load,
    save,
    refresh,
    disconnect,
  };
}
