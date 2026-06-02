"use client";

import { createContext, useContext, useEffect, useState } from "react";
import type { StoreApi } from "zustand";
import { useStore } from "zustand";
import { isDebug, registerSessionTaskResolver } from "@/lib/debug/log";
import type { AppState, StoreProviderProps } from "@/lib/state/store";
import { createAppStore } from "@/lib/state/store";

const StoreContext = createContext<StoreApi<AppState> | null>(null);

type E2EWindow = Window & {
  __KANDEV_E2E_EXPOSE_STORE__?: boolean;
  __KANDEV_E2E_STORE__?: StoreApi<AppState>;
};

export function StateProvider({ children, initialState }: StoreProviderProps) {
  const [store] = useState(() => createAppStore(initialState));

  useEffect(() => {
    const win = window as E2EWindow;
    if (win.__KANDEV_E2E_EXPOSE_STORE__) {
      win.__KANDEV_E2E_STORE__ = store;
    }
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
