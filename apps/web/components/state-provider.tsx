"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useState,
  useSyncExternalStore,
} from "react";
import type { StoreApi } from "zustand";
import { useStore } from "zustand";
import { isDebug, registerSessionTaskResolver } from "@/lib/debug/log";
import type { AppState, StoreProviderProps } from "@/lib/state/store";
import { createAppStore } from "@/lib/state/store";
import { clearQueuedTaskCreateLastUsedIfSynced } from "./task-create-dialog-handlers";

const StoreContext = createContext<StoreApi<AppState> | null>(null);

type E2EWindow = Window & {
  __KANDEV_E2E_EXPOSE_STORE__?: boolean;
  __KANDEV_E2E_STORE__?: StoreApi<AppState>;
};

export function StateProvider({ children, initialState }: StoreProviderProps) {
  const parentStore = useContext(StoreContext);
  const [ownStore] = useState(() => createAppStore(parentStore ? undefined : initialState));
  const store = parentStore ?? ownStore;

  useLayoutEffect(() => {
    if (!parentStore || !initialState || Object.keys(initialState).length === 0) return;
    store.getState().hydrate(initialState);
  }, [initialState, parentStore, store]);

  useEffect(() => {
    const win = window as E2EWindow;
    if (win.__KANDEV_E2E_EXPOSE_STORE__) {
      win.__KANDEV_E2E_STORE__ = store;
    }
  }, [store]);

  useLayoutEffect(() => {
    clearQueuedTaskCreateLastUsed(store.getState());
    return store.subscribe((state, prevState) => {
      if (
        state.userSettings.loaded === prevState.userSettings.loaded &&
        taskCreateLastUsedEqual(
          state.userSettings.taskCreateLastUsed,
          prevState.userSettings.taskCreateLastUsed,
        )
      ) {
        return;
      }
      clearQueuedTaskCreateLastUsed(state);
    });
  }, [store]);

  // In debug builds, let the namespaced debug logger annotate every line that
  // carries a sessionId with `task_id=<...>` so console/log filters can scope to
  // a single task (see lib/debug/log.ts). No-op in production.
  useEffect(() => {
    if (!isDebug()) return;
    return registerSessionTaskResolver(
      (sessionId) => store.getState().taskSessions.items[sessionId]?.task_id,
    );
  }, [store]);

  return <StoreContext.Provider value={store}>{children}</StoreContext.Provider>;
}

function clearQueuedTaskCreateLastUsed(state: AppState) {
  if (!state.userSettings.loaded) return;
  clearQueuedTaskCreateLastUsedIfSynced(state.userSettings.taskCreateLastUsed);
}

function taskCreateLastUsedEqual(
  a: AppState["userSettings"]["taskCreateLastUsed"],
  b: AppState["userSettings"]["taskCreateLastUsed"],
) {
  return (
    a?.repositoryId === b?.repositoryId &&
    a?.branch === b?.branch &&
    a?.agentProfileId === b?.agentProfileId &&
    a?.executorProfileId === b?.executorProfileId &&
    a?.synced === b?.synced
  );
}

export function useAppStore<T>(selector: (state: AppState) => T) {
  const store = useContext(StoreContext);
  if (!store) {
    throw new Error("useAppStore must be used within StateProvider");
  }
  return useStore(store, selector);
}

export function useOptionalAppStore<T>(selector: (state: AppState) => T, fallback: T) {
  const store = useContext(StoreContext);
  const subscribe = useCallback(
    (listener: () => void) => {
      if (!store) return () => {};
      return store.subscribe(() => listener());
    },
    [store],
  );
  const getSnapshot = useCallback(
    () => (store ? selector(store.getState()) : fallback),
    [fallback, selector, store],
  );
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

export function useAppStoreApi() {
  const store = useContext(StoreContext);
  if (!store) {
    throw new Error("useAppStoreApi must be used within StateProvider");
  }
  return store;
}
