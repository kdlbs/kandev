"use client";

import { useLayoutEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { AppState } from "@/lib/state/store";
import { useAppStoreApi } from "@/components/state-provider";
import { seedQueryClientFromInitialState, type QuerySeedInitialState } from "@/lib/query/seed";

type StateHydratorProps = {
  initialState: QuerySeedInitialState;
  /** Session ID to force-merge even if it's active (for navigation refresh) */
  sessionId?: string;
};

export function StateHydrator({ initialState, sessionId }: StateHydratorProps) {
  const store = useAppStoreApi();
  const queryClient = useQueryClient();

  // Use useLayoutEffect to hydrate state synchronously before child effects run.
  // This ensures SSR-hydrated data is available before hooks like useSettingsData
  // decide whether to fetch data.
  useLayoutEffect(() => {
    if (Object.keys(initialState).length) {
      store.getState().hydrate(toStoreInitialState(initialState), {
        forceMergeSessionId: sessionId,
      });
      seedQueryClientFromInitialState(queryClient, initialState, { sessionId });
    }
  }, [initialState, queryClient, sessionId, store]);

  return null;
}

function toStoreInitialState(initialState: QuerySeedInitialState): Partial<AppState> {
  const { workspaces, ...rest } = initialState;
  return {
    ...(rest as Partial<AppState>),
    ...(workspaces ? { workspaces: { activeId: workspaces.activeId ?? null } } : {}),
  };
}
