import { useCallback, useRef } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { listExecutorProfiles } from "@/lib/api/domains/settings-api";

function observeProfiles(store: ReturnType<typeof useAppStoreApi>, executorId: string) {
  let previous = store.getState().executors.items.find((item) => item.id === executorId);
  const changes = { missing: !previous, profiles: false };
  const unsubscribe = store.subscribe((state) => {
    const current = state.executors.items.find((item) => item.id === executorId);
    if (!current) changes.missing = true;
    const before = previous?.profiles ?? [];
    const after = current?.profiles ?? [];
    if (
      before.length !== after.length ||
      before.some((profile, index) => profile !== after[index])
    ) {
      changes.profiles = true;
    }
    previous = current;
  });
  return { changes, unsubscribe };
}

export function useRefreshProfiles(executorId: string) {
  const store = useAppStoreApi();
  const latestRead = useRef(0);
  return useCallback(async () => {
    const read = ++latestRead.current;
    const observation = observeProfiles(store, executorId);
    try {
      const resp = await listExecutorProfiles(executorId, { cache: "no-store" });
      if (
        read !== latestRead.current ||
        observation.changes.missing ||
        observation.changes.profiles
      ) {
        return;
      }
      const current = store.getState().executors.items;
      store
        .getState()
        .setExecutors(
          current.map((item) =>
            item.id === executorId ? { ...item, profiles: resp.profiles } : item,
          ),
        );
    } catch {
      // Refresh failure leaves the current catalogue intact.
    } finally {
      observation.unsubscribe();
    }
  }, [executorId, store]);
}
