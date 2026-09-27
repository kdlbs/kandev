"use client";

import { useLayoutEffect } from "react";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionHydrationEpoch } from "@/lib/state/slices/session/types";
import { useAppStoreApi } from "@/components/state-provider";

type StateHydratorProps = {
  initialState: Partial<AppState>;
  /** Session ID to force-merge even if it's active (for navigation refresh) */
  sessionId?: string;
  /** Session generations captured when the request for this snapshot started. */
  taskSessionHydrationEpochsAtRequestStart?: Readonly<Record<string, TaskSessionHydrationEpoch>>;
};

export function StateHydrator({
  initialState,
  sessionId,
  taskSessionHydrationEpochsAtRequestStart,
}: StateHydratorProps) {
  const store = useAppStoreApi();

  // Use useLayoutEffect to hydrate state synchronously before child effects run.
  // This ensures SSR-hydrated data is available before hooks like useSettingsData
  // decide whether to fetch data.
  useLayoutEffect(() => {
    if (Object.keys(initialState).length) {
      store.getState().hydrate(initialState, {
        forceMergeSessionId: sessionId,
        taskSessionHydrationEpochsAtRequestStart,
      });
    }
  }, [initialState, sessionId, store, taskSessionHydrationEpochsAtRequestStart]);

  return null;
}
