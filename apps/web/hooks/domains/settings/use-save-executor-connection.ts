import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { listExecutors, updateExecutor } from "@/lib/api/domains/settings-api";
import type { Executor } from "@/lib/types/http";

/**
 * Saves an executor's SSH connection form, then refreshes the store so every
 * view of the executor, including its pinned host fingerprint, shows what was
 * saved. The returned callback rejects when the save fails, so a settings save
 * coordinator can report it.
 */
export function useSaveExecutorConnection(
  executorId: string,
  buildConfig: (cfg: SSHExecutorConfig) => Record<string, string>,
  onSaved?: () => void | Promise<void>,
) {
  const store = useAppStoreApi();

  return useCallback(
    async (cfg: SSHExecutorConfig) => {
      const config = buildConfig(cfg);
      await updateExecutor(executorId, { name: cfg.name, config });
      try {
        const fresh = await listExecutors();
        store.getState().setExecutors(fresh.executors);
      } catch {
        // Non-fatal: patch the saved executor into the current snapshot. It is
        // read at write time so a WS update that landed mid-flight is kept.
        const current = store.getState().executors.items;
        store
          .getState()
          .setExecutors(
            current.map((e: Executor) =>
              e.id === executorId ? { ...e, name: cfg.name, config } : e,
            ),
          );
      }
      await onSaved?.();
    },
    [executorId, buildConfig, store, onSaved],
  );
}
