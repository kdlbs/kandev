"use client";

import { createContext, useContext, useEffect, useLayoutEffect, useState } from "react";
import type { StoreApi } from "zustand";
import { useStore } from "zustand";
import { isDebug, registerSessionTaskResolver } from "@/lib/debug/log";
import type { AppState, StoreProviderProps } from "@/lib/state/store";
import { createAppStore } from "@/lib/state/store";
import { removeLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

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
    syncTaskCreateLastUsedCache(store.getState());
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
      syncTaskCreateLastUsedCache(state);
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

function syncTaskCreateLastUsedCache(state: AppState) {
  if (!state.userSettings.loaded) return;
  const lastUsed = state.userSettings.taskCreateLastUsed;
  if (!lastUsed) return;
  if (lastUsed.repositoryId) {
    setLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID, lastUsed.repositoryId);
  } else {
    removeLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID);
  }
  if (lastUsed.branch) {
    setLocalStorage(STORAGE_KEYS.LAST_BRANCH, lastUsed.branch);
  } else {
    removeLocalStorage(STORAGE_KEYS.LAST_BRANCH);
  }
  if (lastUsed.agentProfileId) {
    setLocalStorage(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, lastUsed.agentProfileId);
  } else {
    removeLocalStorage(STORAGE_KEYS.LAST_AGENT_PROFILE_ID);
  }
  if (lastUsed.executorProfileId) {
    setLocalStorage(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, lastUsed.executorProfileId);
  } else {
    removeLocalStorage(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID);
  }
}

function taskCreateLastUsedEqual(
  a: AppState["userSettings"]["taskCreateLastUsed"],
  b: AppState["userSettings"]["taskCreateLastUsed"],
) {
  return (
    a?.repositoryId === b?.repositoryId &&
    a?.branch === b?.branch &&
    a?.agentProfileId === b?.agentProfileId &&
    a?.executorProfileId === b?.executorProfileId
  );
}

export function useAppStore<T>(selector: (state: AppState) => T) {
  const store = useContext(StoreContext);
  if (!store) {
    throw new Error("useAppStore must be used within StateProvider");
  }
  return useStore(store, selector);
}

export function useAppStoreApi() {
  const store = useContext(StoreContext);
  if (!store) {
    throw new Error("useAppStoreApi must be used within StateProvider");
  }
  return store;
}
