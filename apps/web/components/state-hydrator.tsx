"use client";

import { useLayoutEffect } from "react";
import type { AppState } from "@/lib/state/store";
import { useAppStoreApi } from "@/components/state-provider";

type StateHydratorProps = {
  initialState: Partial<AppState>;
  /** Session ID to force-merge even if it's active (for navigation refresh) */
  sessionId?: string;
};

export function StateHydrator({ initialState, sessionId }: StateHydratorProps) {
  const store = useAppStoreApi();

  // Use useLayoutEffect to hydrate state synchronously before child effects run.
  // This ensures SSR-hydrated data is available before hooks like useSettingsData
  // decide whether to fetch data.
  useLayoutEffect(() => {
    if (Object.keys(initialState).length) {
      const before = store.getState().taskPRs;
      console.log("[hydrator] taskPRs in initialState", initialState.taskPRs);
      console.log("[hydrator] taskPRs in store BEFORE hydrate", { byTaskId: Object.keys(before.byTaskId), loaded: before.loaded });
      store.getState().hydrate(initialState, {
        forceMergeSessionId: sessionId,
      });
      const after = store.getState().taskPRs;
      console.log("[hydrator] taskPRs in store AFTER hydrate", { byTaskId: Object.keys(after.byTaskId), loaded: after.loaded });
    }
  }, [initialState, sessionId, store]);

  return null;
}
