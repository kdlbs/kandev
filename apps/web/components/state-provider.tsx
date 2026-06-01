"use client";

import { createContext, useContext, useEffect, useState } from "react";
import type { StoreApi } from "zustand";
import { useStore } from "zustand";
import { IS_DEBUG, registerSessionTaskResolver } from "@/lib/debug/log";
import { getBrowserQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { AppState, StoreProviderProps } from "@/lib/state/store";
import { createAppStore } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

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
    if (!IS_DEBUG) return;
    // Server "taskSessions" state now lives in the TanStack Query cache (the
    // Zustand mirror was removed in the TQ migration). Read the by-id slot the
    // session-state bridge writes (qk.taskSession.byId) to resolve task_id.
    return registerSessionTaskResolver(
      (sessionId) =>
        getBrowserQueryClient().getQueryData<TaskSession | null>(qk.taskSession.byId(sessionId))
          ?.task_id,
    );
  }, []);

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
