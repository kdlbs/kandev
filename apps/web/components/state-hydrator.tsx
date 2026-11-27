"use client";

import { useLayoutEffect, useRef } from "react";
import type { AppState } from "@/lib/state/store";
import type {
  ActiveTurnWindowObservation,
  TaskSessionHydrationEpoch,
} from "@/lib/state/slices/session/types";
import { useAppStoreApi } from "@/components/state-provider";
import type { HydrationOptions } from "@/lib/state/hydration/hydrator";
import { getProvenRemovalPredecessor } from "@/lib/state/session-removal-handoff";
import { useTaskRouteSelection } from "@/hooks/domains/session/use-task-route-selection";

type StateHydratorProps = {
  initialState: Partial<AppState>;
  /** Session ID to force-merge even if it's active (for navigation refresh) */
  sessionId?: string;
  /** Session generations captured when the request for this snapshot started. */
  taskSessionHydrationEpochsAtRequestStart?: Readonly<Record<string, TaskSessionHydrationEpoch>>;
  turnWindowObservationsAtRequestStart?: Readonly<Record<string, ActiveTurnWindowObservation>>;
  taskSelectionAtRequestStart?: HydrationOptions["taskSelectionAtRequestStart"];
  requestedSessionId?: string;
  onHydrated?: () => void;
};

export function StateHydrator({
  initialState,
  sessionId,
  taskSessionHydrationEpochsAtRequestStart,
  turnWindowObservationsAtRequestStart,
  taskSelectionAtRequestStart,
  requestedSessionId,
  onHydrated,
}: StateHydratorProps) {
  const store = useAppStoreApi();
  const admitRouteSelection = useTaskRouteSelection(
    requestedSessionId,
    taskSelectionAtRequestStart,
  );
  const onHydratedRef = useRef(onHydrated);
  onHydratedRef.current = onHydrated;

  // Use useLayoutEffect to hydrate state synchronously before child effects run.
  // This ensures SSR-hydrated data is available before hooks like useSettingsData
  // decide whether to fetch data.
  useLayoutEffect(() => {
    if (Object.keys(initialState).length) {
      admitRouteSelection(initialState);
      const taskId = initialState.tasks?.activeTaskId;
      const preserveTaskSelection =
        !!taskSelectionAtRequestStart &&
        (!!requestedSessionId ||
          (!!taskId &&
            getProvenRemovalPredecessor(store, taskId, store.getState().tasks.activeSessionId) !==
              null));
      store.getState().hydrate(initialState, {
        forceMergeSessionId: sessionId,
        taskSessionHydrationEpochsAtRequestStart,
        turnWindowObservationsAtRequestStart,
        taskSelectionAtRequestStart,
        preserveTaskSelection,
      });
    }
    onHydratedRef.current?.();
  }, [
    initialState,
    sessionId,
    store,
    taskSessionHydrationEpochsAtRequestStart,
    turnWindowObservationsAtRequestStart,
    taskSelectionAtRequestStart,
    requestedSessionId,
    admitRouteSelection,
  ]);

  return null;
}
