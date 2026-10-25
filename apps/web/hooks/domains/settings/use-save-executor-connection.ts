import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { listExecutors, updateExecutor } from "@/lib/api/domains/settings-api";
import type { Executor } from "@/lib/types/http";

type ConnectionChanges = { name: boolean; keys: Set<string>; missing: boolean };
type ConnectionValues = Pick<Executor, "name" | "config">;

function observeConnection(store: ReturnType<typeof useAppStoreApi>, executorId: string) {
  let previous = store.getState().executors.items.find((item) => item.id === executorId);
  const changes: ConnectionChanges = { name: false, keys: new Set(), missing: !previous };
  const unsubscribe = store.subscribe((state) => {
    const current = state.executors.items.find((item) => item.id === executorId);
    if (!current) changes.missing = true;
    if (previous && current) {
      if (previous.name !== current.name) changes.name = true;
      const before = previous.config ?? {};
      const after = current.config ?? {};
      for (const key of new Set([...Object.keys(before), ...Object.keys(after)])) {
        if (
          before[key] !== after[key] ||
          Object.hasOwn(before, key) !== Object.hasOwn(after, key)
        ) {
          changes.keys.add(key);
        }
      }
    }
    previous = current;
  });
  return { changes, unsubscribe };
}

function mergeConnection(
  current: Executor,
  selected: ConnectionValues,
  changes: ConnectionChanges,
) {
  const config = current.config ?? {};
  const accepted = selected.config ?? {};
  const keys = new Set([...Object.keys(config), ...Object.keys(accepted)]);
  const entries = [...keys].flatMap((key) => {
    const source = changes.keys.has(key) ? config : accepted;
    return Object.hasOwn(source, key) ? [[key, source[key]]] : [];
  });
  return {
    ...current,
    name: changes.name ? current.name : selected.name,
    config: Object.fromEntries(entries),
  };
}

/** Saves a connection and refreshes its untouched fields, preserving live catalogue changes. */
export function useSaveExecutorConnection(
  executorId: string,
  buildConfig: (cfg: SSHExecutorConfig) => Record<string, string>,
  onSaved?: () => void | Promise<void>,
) {
  const store = useAppStoreApi();

  return useCallback(
    async (cfg: SSHExecutorConfig) => {
      const config = buildConfig(cfg);
      const observation = observeConnection(store, executorId);
      try {
        await updateExecutor(executorId, { name: cfg.name, config });
        let selected: ConnectionValues = { name: cfg.name, config };
        try {
          const fresh = await listExecutors();
          selected = fresh.executors.find((item) => item.id === executorId) ?? selected;
        } catch {
          // Refresh failure leaves submitted values as the fallback for untouched fields.
        }
        if (!observation.changes.missing) {
          const current = store.getState().executors.items;
          store
            .getState()
            .setExecutors(
              current.map((item) =>
                item.id === executorId
                  ? mergeConnection(item, selected, observation.changes)
                  : item,
              ),
            );
        }
      } finally {
        observation.unsubscribe();
      }
      await onSaved?.();
    },
    [executorId, buildConfig, store, onSaved],
  );
}
